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
	MB = 1 << 20
)

var (
	// Define boot arguments.
	argsPort           = flag.Int("p", 8888 /*     */, "[optional] port")
	argsFileExpiration = flag.Int("e", 10 /*       */, "[optional] default file expiration (minutes)")
	argsMaxFileSize    = flag.Int64("m", 1024 /*   */, "[optional] max file size (MB)")
	argsHelp           = flag.Bool("h", false /*   */, "\nhelp")
	// Define application logic variables.
	fileRegistryMap = map[string]FileRegistry{}
	mutex           sync.Mutex
	logger          = log.New(os.Stdout, "[Logger] ", log.Llongfile|log.LstdFlags)
)

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
	// 時刻と時刻のマイクロ秒、ディレクトリパスを含めたファイル名を出力
	log.SetFlags(log.Llongfile | log.LstdFlags)
}

func main() {

	//-------------------------
	// 引数のパース
	flag.Parse()
	if *argsHelp {
		flag.Usage()
		os.Exit(0)
	}

	//-------------------------
	// 各種パスの処理
	// upload
	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		logger.Println(r.RemoteAddr, r.RequestURI, r.Header)
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
		logger.Println(responseBody)
		responseJson(w, 200, responseBody)
	})

	// download
	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		logger.Println(r.RemoteAddr, r.RequestURI, r.Header)
		if allowedHttpMethod := http.MethodGet; r.Method != allowedHttpMethod {
			responseJson(w, 405, `{"message":"Method Not Allowed. (Only `+allowedHttpMethod+` is allowed)"}`)
			return
		}
		key := r.URL.Query().Get("key")
		logger.Println("key:" + key)
		mutex.Lock()
		defer func() { mutex.Unlock() }()
		if _, ok := fileRegistryMap[key]; !ok {
			responseJson(w, 404, `{"message":"file not found."}`)
			return
		}
		logger.Println(fileRegistryMap[key].String())
		w.WriteHeader(200)
		w.Header().Set("Content-Type", fileRegistryMap[key].multipartFileHeader.Header.Get("Content-Type"))
		w.Header().Set("Content-Disposition", "attachment; filename="+fileRegistryMap[key].multipartFileHeader.Filename)
		io.Copy(w, fileRegistryMap[key].multipartFile)
		// reset
		fileRegistryMap[key].multipartFile.Seek(0, io.SeekStart)
	})

	// 期限切れのファイルを削除するgoroutine
	go func() {
		for {
			time.Sleep(time.Minute * 1)
			func() {
				mutex.Lock()
				defer func() { mutex.Unlock() }()
				for key, fileRegistry := range fileRegistryMap {
					if fileRegistry.expiredAt.Before(time.Now()) {
						logger.Println("[File cleaner goroutine] File expired. >> " + fileRegistry.String())
						delete(fileRegistryMap, key)
					}
				}
			}()
		}
	}()

	//-------------------------
	// Listen開始
	var err error
	logger.Printf("server(http) %d\n", *argsPort)
	logger.Println(`Start application:
######## ######## ##     ## ########     ######## #### ##       ########    ########  ########  ######   ####  ######  ######## ########  ##    ## 
   ##    ##       ###   ### ##     ##    ##        ##  ##       ##          ##     ## ##       ##    ##   ##  ##    ##    ##    ##     ##  ##  ##  
   ##    ##       #### #### ##     ##    ##        ##  ##       ##          ##     ## ##       ##         ##  ##          ##    ##     ##   ####   
   ##    ######   ## ### ## ########     ######    ##  ##       ######      ########  ######   ##   ####  ##   ######     ##    ########     ##    
   ##    ##       ##     ## ##           ##        ##  ##       ##          ##   ##   ##       ##    ##   ##        ##    ##    ##   ##      ##    
   ##    ##       ##     ## ##           ##        ##  ##       ##          ##    ##  ##       ##    ##   ##  ##    ##    ##    ##    ##     ##    
   ##    ######## ##     ## ##           ##       #### ######## ########    ##     ## ########  ######   ####  ######     ##    ##     ##    ##`)
	err = http.ListenAndServe(":"+strconv.Itoa(*argsPort), nil)
	if err != nil {
		logger.Fatal("ListenAndServe: ", err)
	}
}

func responseJson(w http.ResponseWriter, statusCode int, bodyJson string) {
	w.WriteHeader(statusCode)
	w.Header().Add("Content-Type", "application/json")
	fmt.Fprintf(w, bodyJson)
}
