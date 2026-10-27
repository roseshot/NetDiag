package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/roseshot/NetDiag/internal/tcpwindow"
)

func TestTCPWindowSessionAndTransfer(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("Linux TCP_INFO session only") }
	handler, a := newHandler(nil)
	var counter atomic.Uint32
	counter.Store(10)
	a.tcpWindows = tcpwindow.New(func(net.Conn) (uint32, error) {
		return counter.Load(), nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/diag/tcp/window", nil)
	req.RemoteAddr = "192.0.2.5:5555"
	req.Host = "netdiag.example.test"
	req.Header.Set("Origin", "http://netdiag.example.test")
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, req)
	if out.Code != http.StatusOK { t.Fatalf("start returned %d: %s",out.Code,out.Body.String()) }
	var created struct { Token string `json:"token"` }
	if err:=json.Unmarshal(out.Body.Bytes(),&created);err!=nil{t.Fatal(err)}
	if len(created.Token)!=39 { t.Fatalf("unexpected token length %d",len(created.Token)) }

	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	testReq := httptest.NewRequest(http.MethodPost, "/api/upload", bytes.NewReader(make([]byte,1024)))
	testReq.RemoteAddr = "192.0.2.5:6789"
	testReq.Header.Set("X-NetDiag-Window",created.Token)
	testReq = testReq.WithContext(context.WithValue(testReq.Context(),connectionKey{},conn))
	counter.Store(12)
	transfer := httptest.NewRecorder()
	handler.ServeHTTP(transfer,testReq)
	if transfer.Code!=http.StatusOK { t.Fatalf("transfer failed: %d",transfer.Code) }

	cross := httptest.NewRequest(http.MethodGet,"/api/diag/tcp/window?token="+created.Token,nil)
	cross.RemoteAddr = "198.51.100.77:4500"
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing,cross)
	if missing.Code!=http.StatusNotFound { t.Fatalf("unauthorized read: %d",missing.Code) }

	r := httptest.NewRequest(http.MethodGet,"/api/diag/tcp/window?token="+created.Token,nil)
	r.RemoteAddr="192.0.2.5:9876"
	result := httptest.NewRecorder()
	handler.ServeHTTP(result,r)
	if result.Code!=http.StatusOK {t.Fatalf("result status: %d",result.Code)}
	var actual tcpwindow.Result
	if err:=json.Unmarshal(result.Body.Bytes(),&actual);err!=nil{t.Fatal(err)}
	if actual.Status!="completed"||actual.Connections!=1||actual.CompletedRequests!=1 {
		t.Fatalf("incorrect TCP observation: %+v",actual)
	}
	// The synthetic sample returns the same counter at begin and finish.
	// A real counter may change while a request is active.
	if actual.Retransmissions!=0 {t.Fatalf("unexpected delta: %d",actual.Retransmissions)}
	if actual.Scope!="server_tcp_sockets" {t.Fatalf("incorrect measurement scope: %q",actual.Scope)}
}

func TestTCPWindowSessionSecurity(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("Linux TCP_INFO session only") }
	handler,_:=newHandler(nil)
	for _,tc:=range []struct{
		method string
		origin string
		fetchSite string
		want int
	}{
		{http.MethodPost,"https://other.example","same-origin",http.StatusForbidden},
		{http.MethodPost,"","cross-site",http.StatusForbidden},
		{http.MethodDelete,"","",http.StatusMethodNotAllowed},
	}{
		req:=httptest.NewRequest(tc.method,"/api/diag/tcp/window",nil)
		req.Host="netdiag.example.test"
		if tc.origin!="" {req.Header.Set("Origin",tc.origin)}
		if tc.fetchSite!="" {req.Header.Set("Sec-Fetch-Site",tc.fetchSite)}
		result:=httptest.NewRecorder()
		handler.ServeHTTP(result,req)
		if result.Code!=tc.want {t.Errorf("method %s origin %q: %d, want %d",tc.method,tc.origin,result.Code,tc.want)}
	}
	r:=httptest.NewRequest(http.MethodGet,"/api/diag/tcp/window?token=wrong",nil)
	res:=httptest.NewRecorder()
	handler.ServeHTTP(res,r)
	if res.Code!=http.StatusNotFound {t.Fatalf("invalid token: %d",res.Code)}
}

func TestTCPWindowMissingSocketReportsUnavailable(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("Linux TCP_INFO session only") }
	handler,_:=newHandler(nil)
	start:=httptest.NewRecorder()
	req:=httptest.NewRequest(http.MethodPost,"/api/diag/tcp/window",nil)
	req.RemoteAddr="192.0.2.8:1234"
	handler.ServeHTTP(start,req)
	if start.Code!=http.StatusOK {t.Fatalf("start: %d",start.Code)}
	var result struct{Token string}
	if err:=json.Unmarshal(start.Body.Bytes(),&result);err!=nil {t.Fatal(err)}
	read:=httptest.NewRequest(http.MethodGet,"/api/diag/tcp/window?token="+result.Token,nil)
	read.RemoteAddr="192.0.2.8:1234"
	out:=httptest.NewRecorder()
	handler.ServeHTTP(out,read)
	if out.Code!=http.StatusOK||!strings.Contains(out.Body.String(),`"status":"unavailable"`) {
		t.Fatalf("no accepted TCP socket must not report zero retransmissions: %d %s",out.Code,out.Body.String())
	}
}
