package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Size constants
const (
	MB                 = 1 << 20
	CommandDescription = "temp-file-registry is temporary file registry provided through an HTTP web API."
	UsageDummy         = "########"
	UrlPathPrefix      = "/temp-file-registry/api/v1"
)

var (
	// Define boot arguments.
	//
	argsPort           = defineIntParam("p", "port", "[optional] Port", 8888)
	argsFileExpiration = defineIntParam("e", "expiration-minutes", "[optional] Default file expiration (minutes)", 10)
	argsMaxFileSize    = defineInt64Param("m", "max-file-size-mb", "[optional] Max file size (MB)", 1024)
	argsLogLevel       = defineIntParam("l", "log-level", "[optional] Log level (0:Panic, 1:Info, 2:Debug)", 2)
	argsHelp           = defineBoolParam("h", "help", "help")
	// Define application logic variables.
	fileRegistryMap = map[string]FileRegistry{}
	mutex           sync.Mutex
	// Define logger: date, time, microseconds, directory and file path are always outputted.
	logger         = log.New(os.Stdout, "[Logger] ", log.Llongfile|log.LstdFlags)
	loggerLogLevel = Debug
)

type LogLevel int

const (
	Panic LogLevel = iota
	Info
	Debug
)

// Level based logging in Golang https://www.linkedin.com/pulse/level-based-logging-golang-vivek-dasgupta
func logging(loglevel LogLevel, logLogger *log.Logger, v ...interface{}) {
	if loggerLogLevel < loglevel {
		return
	}
	level := func() string {
		switch loggerLogLevel {
		case Panic:
			return "Panic"
		case Info:
			return "Info"
		case Debug:
			return "Debug"
		default:
			return "Unknown"
		}
	}()
	logLogger.Println(append([]interface{}{"[" + level + "]"}, v...)...)
	if loggerLogLevel == Panic {
		logLogger.Panic(v...)
	}
}

type FileRegistry struct {
	key                 string
	expiryTimeMinutes   string
	expiredAt           time.Time
	multipartFile       multipart.File
	multipartFileHeader *multipart.FileHeader
}

func (fr FileRegistry) String() string {
	return fmt.Sprintf("key:%v, expiryTimeMinutes:%v, expiredAt:%v, multipartFileHeader.Header:%v", fr.key, fr.expiryTimeMinutes, fr.expiredAt, fr.multipartFileHeader.Header)
}

func init() {
	adjustUsage()
}

func main() {

	//-------------------------
	// 引数のパース
	flag.Parse()
	if *argsHelp {
		flag.Usage()
		os.Exit(0)
	}
	// set log level
	loggerLogLevel = LogLevel(*argsLogLevel)

	//-------------------------
	// 各パスの処理
	// upload
	http.HandleFunc(UrlPathPrefix+"/upload", handleUpload)
	// download
	http.HandleFunc(UrlPathPrefix+"/download", handleDownload)

	//-------------------------
	// 期限切れのファイルを削除するgoroutineを起動
	go cleanExpiredFile()

	//-------------------------
	// Listen開始
	logging(Info, logger, "server(http)", *argsPort)
	logging(Info, logger, `Start application:
######## ######## ##     ## ########     ######## #### ##       ########    ########  ########  ######   ####  ######  ######## ########  ##    ## 
   ##    ##       ###   ### ##     ##    ##        ##  ##       ##          ##     ## ##       ##    ##   ##  ##    ##    ##    ##     ##  ##  ##  
   ##    ##       #### #### ##     ##    ##        ##  ##       ##          ##     ## ##       ##         ##  ##          ##    ##     ##   ####   
   ##    ######   ## ### ## ########     ######    ##  ##       ######      ########  ######   ##   ####  ##   ######     ##    ########     ##    
   ##    ##       ##     ## ##           ##        ##  ##       ##          ##   ##   ##       ##    ##   ##        ##    ##    ##   ##      ##    
   ##    ##       ##     ## ##           ##        ##  ##       ##          ##    ##  ##       ##    ##   ##  ##    ##    ##    ##    ##     ##    
   ##    ######## ##     ## ##           ##       #### ######## ########    ##     ## ########  ######   ####  ######     ##    ##     ##    ##`)
	if err := http.ListenAndServe(":"+strconv.Itoa(*argsPort), nil); err != nil {
		logging(Panic, logger, err)
	}
}

// Upload処理：POSTされたファイルをアプリ内部のmapに保持する
func handleUpload(w http.ResponseWriter, r *http.Request) {
	logging(Debug, logger, r.RemoteAddr, r.RequestURI, r.Header)
	// - [How can I handle http requests of different methods to / in Go? - Stack Overflow](https://stackoverflow.com/questions/15240884/how-can-i-handle-http-requests-of-different-methods-to-in-go)
	if allowedHttpMethod := http.MethodPost; r.Method != allowedHttpMethod {
		responseJson(w, 405, `{"message":"Method Not Allowed. (Only `+allowedHttpMethod+` is allowed)"}`)
		return
	}

	// go - golang - How to check multipart.File information - Stack Overflow
	// https://stackoverflow.com/questions/17129797/golang-how-to-check-multipart-file-information
	if err := r.ParseMultipartForm(*argsMaxFileSize * MB); err != nil {
		responseJson(w, 400, `{"message":"`+err.Error()+`"}`)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, *argsMaxFileSize*MB)

	key := r.FormValue("key")
	expiryTimeMinutes := r.FormValue("expiryTimeMinutes")
	fileExpirationMinutes := *argsFileExpiration
	// expiryTimeMinutesで指定された数値がintにキャストできる値だった場合は、ファイルの期限を指定分に設定する
	if expiryTimeMinutesInt, err := strconv.Atoi(expiryTimeMinutes); err == nil {
		fileExpirationMinutes = expiryTimeMinutesInt
	}
	file, fileHeader, _ := r.FormFile("file")
	mutex.Lock()
	defer func() { mutex.Unlock() }()
	fileRegistryMap[key] = FileRegistry{
		key:                 key,
		expiryTimeMinutes:   expiryTimeMinutes,
		expiredAt:           time.Now().Add(time.Duration(fileExpirationMinutes) * time.Minute),
		multipartFile:       file,
		multipartFileHeader: fileHeader,
	}
	responseBody := `{"message":"` + fileRegistryMap[key].String() + `"}`
	logging(Debug, logger, responseBody)
	responseJson(w, 200, responseBody)
}

// Download処理：key指定されたファイルをレスポンスする
func handleDownload(w http.ResponseWriter, r *http.Request) {
	logging(Debug, logger, r.RemoteAddr, r.RequestURI, r.Header)
	if allowedHttpMethod := http.MethodGet; r.Method != allowedHttpMethod {
		responseJson(w, 405, `{"message":"Method Not Allowed. (Only `+allowedHttpMethod+` is allowed)"}`)
		return
	}
	key := r.URL.Query().Get("key")
	deleteFlag := r.URL.Query().Get("delete")
	mutex.Lock()
	defer func() { mutex.Unlock() }()
	if _, ok := fileRegistryMap[key]; !ok {
		responseJson(w, 404, `{"message":"file not found."}`)
		return
	}
	logging(Debug, logger, fileRegistryMap[key].String())
	w.WriteHeader(200)
	w.Header().Set("Content-Type", fileRegistryMap[key].multipartFileHeader.Header.Get("Content-Type"))
	w.Header().Set("Content-Disposition", "attachment; filename="+fileRegistryMap[key].multipartFileHeader.Filename)
	io.Copy(w, fileRegistryMap[key].multipartFile)
	// reset
	fileRegistryMap[key].multipartFile.Seek(0, io.SeekStart)
	// if specified "delete" parameter, target file will be deleted after response.
	if deleteFlag == "true" {
		delete(fileRegistryMap, key)
	}
}

// 期限切れのファイルをお掃除する
func cleanExpiredFile() {
	for {
		time.Sleep(time.Minute * 1)
		func() {
			mutex.Lock()
			defer func() { mutex.Unlock() }()
			for key, fileRegistry := range fileRegistryMap {
				if fileRegistry.expiredAt.Before(time.Now()) {
					logging(Debug, logger, "[File cleaner goroutine] File expired. >>", fileRegistry.String())
					delete(fileRegistryMap, key)
				}
			}
		}()
	}
}

func responseJson(w http.ResponseWriter, statusCode int, bodyJson string) {
	w.WriteHeader(statusCode)
	w.Header().Add("Content-Type", "application/json")
	fmt.Fprint(w, bodyJson)
}

// =======================================
// Internal Utils
// =======================================

func defineIntParam(short, long, description string, defaultValue int) (v *int) {
	v = flag.Int(short, 0, UsageDummy)
	flag.IntVar(v, long, defaultValue, description)
	return
}

func defineInt64Param(short, long, description string, defaultValue int64) (v *int64) {
	v = flag.Int64(short, 0, UsageDummy)
	flag.Int64Var(v, long, defaultValue, description)
	return
}

func defineBoolParam(short, long, description string) (v *bool) {
	v = flag.Bool(short, false, UsageDummy)
	flag.BoolVar(v, long, false, description)
	return
}

func adjustUsage() {
	// Get default flags usage
	b := new(bytes.Buffer)
	func() { flag.CommandLine.SetOutput(b); flag.Usage(); flag.CommandLine.SetOutput(os.Stderr) }()
	// Get default flags usage
	re := regexp.MustCompile("(-\\S+)( *\\S*)+\n*\\s+" + UsageDummy + "\n*\\s+(-\\S+)( *\\S*)+\n\\s+(.+)")
	usageParams := re.FindAllString(b.String(), -1)
	maxLengthParam := 0.0
	sort.Slice(usageParams, func(i, j int) bool {
		maxLengthParam = math.Max(maxLengthParam, math.Max(float64(len(re.ReplaceAllString(usageParams[i], "$1, -$3$4"))), float64(len(re.ReplaceAllString(usageParams[j], "$1, -$3$4")))))
		return strings.Compare(usageParams[i], usageParams[j]) == -1
	})
	usage := strings.Replace(strings.Replace(strings.Split(b.String(), "\n")[0], ":", " [OPTIONS]", -1), " of ", ": ", -1) + "\n\nDescription:\n  " + CommandDescription + "\n\nOptions:\n"
	for _, v := range usageParams {
		usage += fmt.Sprintf("%-6s%-"+strconv.Itoa(int(maxLengthParam))+"s", re.ReplaceAllString(v, "  $1,"), re.ReplaceAllString(v, "-$3$4")) + re.ReplaceAllString(v, "$5\n")
	}
	flag.Usage = func() { _, _ = fmt.Fprintf(flag.CommandLine.Output(), usage) }
}
