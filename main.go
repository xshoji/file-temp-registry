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
	expiredAt           time.Time
	multipartFile       multipart.File
	multipartFileHeader *multipart.FileHeader
}

func init() {
	// 時刻と時刻のマイクロ秒、ディレクトリパスを含めたファイル名を出力
	log.SetFlags(log.Llongfile | log.LstdFlags)
}

func main() {

	//-------------------------
	// 引数のパース
	flag.Parse()
	// Required parameter
	// - [Can Go's `flag` package print usage? - Stack Overflow](https://stackoverflow.com/questions/23725924/can-gos-flag-package-print-usage)
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
		file, fileHeader, _ := r.FormFile("file")
		logger.Println("key:"+key, ",", "expiryTimeMinutes:"+expiryTimeMinutes, ",", fileHeader.Header)

		mutex.Lock()
		defer func() { mutex.Unlock() }()
		fileRegistryMap[key] = FileRegistry{
			expiredAt:           time.Now().Add(time.Duration(*argsFileExpiration) * time.Minute),
			multipartFile:       file,
			multipartFileHeader: fileHeader,
		}
		responseJson(w, 200, `{"message":"key:`+key+`, expiryTimeMinutes:`+expiryTimeMinutes+`, fileHeader:`+fmt.Sprintf("%s", fileHeader.Header)+`"}`)
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
		w.WriteHeader(200)
		w.Header().Add("Content-Type", fileRegistryMap[key].multipartFileHeader.Header.Get("Content-Type"))
		w.Header().Add("Content-Disposition", fileRegistryMap[key].multipartFileHeader.Header.Get("Content-Disposition"))
		io.Copy(w, fileRegistryMap[key].multipartFile)
	})

	// 期限切れのファイルを削除するgoroutine
	go func() {
		for {
			time.Sleep(time.Minute * 1)
			func() {
				mutex.Lock()
				defer func() { mutex.Unlock() }()
				for key, fileRegistry := range fileRegistryMap {
					if fileRegistry.expiredAt.After(time.Now()) {
						logger.Println("[File cleaner goroutine] File expired. >> key:"+key, ",", "expiredAt:"+fileRegistry.expiredAt.String(), ",", fileRegistry.multipartFileHeader.Header)
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
######## #### ##       ########    ######## ######## ##     ## ########     ########  ########  ######   ####  ######  ######## ########  ##    ## 
##        ##  ##       ##             ##    ##       ###   ### ##     ##    ##     ## ##       ##    ##   ##  ##    ##    ##    ##     ##  ##  ##  
##        ##  ##       ##             ##    ##       #### #### ##     ##    ##     ## ##       ##         ##  ##          ##    ##     ##   ####   
######    ##  ##       ######         ##    ######   ## ### ## ########     ########  ######   ##   ####  ##   ######     ##    ########     ##    
##        ##  ##       ##             ##    ##       ##     ## ##           ##   ##   ##       ##    ##   ##        ##    ##    ##   ##      ##    
##        ##  ##       ##             ##    ##       ##     ## ##           ##    ##  ##       ##    ##   ##  ##    ##    ##    ##    ##     ##    
##       #### ######## ########       ##    ######## ##     ## ##           ##     ## ########  ######   ####  ######     ##    ##     ##    ##    
`)
	err = http.ListenAndServe(":"+strconv.Itoa(*argsPort), nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func responseJson(w http.ResponseWriter, statusCode int, bodyJson string) {
	w.WriteHeader(statusCode)
	w.Header().Add("Content-Type", "application/json")
	fmt.Fprintf(w, bodyJson)
}
