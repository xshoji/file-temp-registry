## Temp file registry

Temporal file registry written by golang.

```
# help
go run main.go -h
Usage of /var/folders/_q/dpw924t12bj25568xfxcd2wm0000gn/T/go-build570030480/b001/exe/main:
  -e int
    	[optional] Default file expiration (minutes) (default 10)
  -h
    	help
  -l int
    	[optional] Log level (0:Panic, 1:Info, 2:Debug) (default 2)
  -m int
    	[optional] Max file size (MB) (default 1024)
  -p int
    	[optional] Port (default 8888)

# execute
go run main.go


# build
APP=/tmp/tfr; go build -ldflags="-s -w" -o ${APP} main.go; chmod +x ${APP}
# APP=/tmp/tfr; GOOS=linux GOARCH=amd64   go build -ldflags="-s -w" -o ${APP} main.go; chmod +x ${APP} # linux
# APP=/tmp/tfr; GOOS=darwin GOARCH=amd64  go build -ldflags="-s -w" -o ${APP} main.go; chmod +x ${APP} # macOS
# APP=/tmp/tfr; GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ${APP} main.go; chmod +x ${APP} # windows


# start
/tmp/tfr
```

## API

### Upload

```
curl --location --request POST 'http://localhost:8888/temp-file-registry/api/v1/upload' \
--form 'key="kioveyzrrt287opddhk9"' \
--form 'file=@"/private/tmp/app"'
{"message":"key:kioveyzrrt287opddhk9, expiryTimeMinutes:10, fileHeader:map[Content-Disposition:[form-data; name="file"; filename="app"] Content-Type:[application/octet-stream]]"}
```

### Download

```
# delete: if "true" specified, target file will be deleted after response.
curl "http://localhost:8888/temp-file-registry/api/v1/download?key=kioveyzrrt287opddhk9&delete=true" -o /tmp/app2
```
