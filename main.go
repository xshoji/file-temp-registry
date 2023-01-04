package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Size constants
const (
	MB            = 1 << 20
	UrlPathPrefix = "/tempFileRegistry/api/v1"
)

var (
	// Define boot arguments.
	argsPort           = flag.Int("p", 8888 /*   */, "[optional] Port")
	argsFileExpiration = flag.Int("e", 10 /*     */, "[optional] Default file expiration (minutes)")
	argsMaxFileSize    = flag.Int64("m", 1024 /* */, "[optional] Max file size (MB)")
	argsLogLevel       = flag.Int("l", 2 /*      */, "[optional] Log level (0:Panic, 1:Info, 2:Debug)")
	argsHelp           = flag.Bool("h", false /* */, "\nhelp")
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
	// 何もしない
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
	fmt.Fprintf(w, bodyJson)
}
