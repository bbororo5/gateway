package httpstats

import (
	"bufio"
	"net"
	"net/http"
)

type Response struct {
	http.ResponseWriter
	Status int
	Bytes  int64
}

func (r *Response) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *Response) WriteHeader(status int) {
	if r.Status != 0 {
		return
	}
	r.ResponseWriter.WriteHeader(status)
	if status >= 200 || status == http.StatusSwitchingProtocols {
		r.Status = status
	}
}

func (r *Response) Write(p []byte) (int, error) {
	if r.Status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(p)
	r.Bytes += int64(n)
	return n, err
}

func (r *Response) FlushError() error {
	if r.Status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *Response) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, buffers, err := http.NewResponseController(r.ResponseWriter).Hijack()
	if err == nil && r.Status == 0 {
		r.Status = http.StatusSwitchingProtocols
	}
	return conn, buffers, err
}

func (r *Response) StatusCode() int {
	if r.Status == 0 {
		return http.StatusOK
	}
	return r.Status
}
