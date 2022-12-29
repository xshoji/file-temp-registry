## Temp file registry

Temporal file registry written by golang.

```
# help
go run main.go -h
Usage of /var/folders/_q/dpw924t12bj25568xfxcd2wm0000gn/T/go-build570030480/b001/exe/main:
  -e int
    	[optional] default file expiration (minutes) (default 10)
  -h
    	help
  -m int
    	[optional] max file size (MB) (default 1024)
  -p int
    	[optional] port (default 8888)


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
curl --location --request POST 'http://localhost:8888/tempFileRegistry/api/v1/upload' \
> --form 'key="kioveyzrrt287opddhk9"' \
> --form 'file=@"/private/tmp/app"'
{"message":"key:kioveyzrrt287opddhk9, expiryTimeMinutes:10, fileHeader:map[Content-Disposition:[form-data; name="file"; filename="app"] Content-Type:[application/octet-stream]]"}
```

### Download

```
curl http://localhost:8888/tempFileRegistry/api/v1/download?key=kioveyzrrt287opddhk9 -o /tmp/app2
```
