package resource

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	mihomoHttp "github.com/metacubex/mihomo/component/http"
	"github.com/metacubex/mihomo/component/profile/cachefile"
	types "github.com/metacubex/mihomo/constant/provider"
)

const (
	DefaultHttpTimeout = time.Second * 20

	fileMode os.FileMode = 0o666
	dirMode  os.FileMode = 0o755
)

func safeWrite(path string, buf []byte) error {
	dir := filepath.Dir(path)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return err
		}
	}

	return os.WriteFile(path, buf, fileMode)
}

type FileVehicle struct {
	path string
}

func (f *FileVehicle) Type() types.VehicleType {
	return types.File
}

func (f *FileVehicle) Path() string {
	return f.path
}

func (f *FileVehicle) Url() string {
	return "file://" + f.path
}

func (f *FileVehicle) Read(ctx context.Context, oldHash types.HashType) (buf []byte, hash types.HashType, err error) {
	buf, err = os.ReadFile(f.path)
	if err != nil {
		return
	}
	hash = types.MakeHash(buf)
	return
}

func (f *FileVehicle) Proxy() string {
	return ""
}

func (f *FileVehicle) Write(buf []byte) error {
	return safeWrite(f.path, buf)
}

func NewFileVehicle(path string) *FileVehicle {
	return &FileVehicle{path: path}
}

type HTTPVehicle struct {
	url     string
	path    string
	proxy   string
	header  http.Header
	timeout time.Duration
}

func (h *HTTPVehicle) Url() string {
	return h.url
}

func (h *HTTPVehicle) Type() types.VehicleType {
	return types.HTTP
}

func (h *HTTPVehicle) Path() string {
	return h.path
}

func (h *HTTPVehicle) Proxy() string {
	return h.proxy
}

func (h *HTTPVehicle) Write(buf []byte) error {
	return safeWrite(h.path, buf)
}

func (h *HTTPVehicle) Read(ctx context.Context, oldHash types.HashType) (buf []byte, hash types.HashType, err error) {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	header := h.header
	setIfNoneMatch := false
	if oldHash.IsValid() {
		hashBytes, etag := cachefile.Cache().GetETagWithHash(h.url)
		if oldHash.EqualBytes(hashBytes) && etag != "" {
			if header == nil {
				header = http.Header{}
			} else {
				header = header.Clone()
			}
			header.Set("If-None-Match", etag)
			setIfNoneMatch = true
		}
	}
	resp, err := mihomoHttp.HttpRequestWithProxy(ctx, h.url, http.MethodGet, header, nil, h.proxy)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if setIfNoneMatch && resp.StatusCode == http.StatusNotModified {
			return nil, oldHash, nil
		}
		err = errors.New(resp.Status)
		return
	}
	buf, err = io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	hash = types.MakeHash(buf)
	cachefile.Cache().SetETagWithHash(h.url, hash.Bytes(), resp.Header.Get("ETag"))
	return
}

func NewHTTPVehicle(url string, path string, proxy string, header http.Header, timeout time.Duration) *HTTPVehicle {
	return &HTTPVehicle{
		url:     url,
		path:    path,
		proxy:   proxy,
		header:  header,
		timeout: timeout,
	}
}
