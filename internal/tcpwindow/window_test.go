package tcpwindow

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDistinctSocketsDoNotDoubleCountOverlappingRequests(t *testing.T) {
	var counter atomic.Uint32
	counter.Store(100)
	m := New(func(net.Conn) (uint32, error) { return counter.Load(), nil })
	token, _, err := m.Start("192.0.2.1")
	if err != nil { t.Fatal(err) }
	conn1, remote1 := net.Pipe()
	defer conn1.Close()
	defer remote1.Close()
	conn2, remote2 := net.Pipe()
	defer conn2.Close()
	defer remote2.Close()

	// Two simultaneous requests on the same HTTP/2-like socket share
	// the original baseline, rather than summing overlapping intervals.
	end1 := m.Begin(token, "192.0.2.1", conn1)
	end2 := m.Begin(token, "192.0.2.1", conn1)
	if end1 == nil || end2 == nil { t.Fatal("missing tagged transfer observation") }
	counter.Store(102)
	end1()
	if snapshot, ok := m.Read(token, "192.0.2.1"); !ok || snapshot.Status != "pending" {
		t.Fatalf("active transfer should be pending: %+v, ok=%v", snapshot, ok)
	}
	counter.Store(105)
	end2()
	endOther := m.Begin(token, "192.0.2.1", conn2)
	if endOther == nil { t.Fatal("second socket not observed") }
	counter.Store(108)
	endOther()
	result, ok := m.Read(token, "192.0.2.1")
	if !ok || result.Status != "completed" || result.Connections != 2 ||
		result.Retransmissions != 8 || result.CompletedRequests != 3 {
		t.Fatalf("unexpected window result %+v, found=%t", result, ok)
	}
	if _, found := m.Read(token, "192.0.2.1"); found {
		t.Fatal("completed token should be consumed")
	}
}

func TestCounterWrapAround(t *testing.T) {
	var counter atomic.Uint32
	counter.Store(^uint32(0)-1)
	m := New(func(net.Conn) (uint32, error) { return counter.Load(), nil })
	token, _, err := m.Start("192.0.2.1")
	if err != nil { t.Fatal(err) }
	conn, other := net.Pipe()
	defer conn.Close()
	defer other.Close()
	finish := m.Begin(token, "192.0.2.1", conn)
	counter.Store(2)
	finish()
	got, _ := m.Read(token, "192.0.2.1")
	if got.Retransmissions != 4 { t.Fatalf("wrap delta = %d, want 4", got.Retransmissions) }
}

func TestSessionQuotaPeerIsolationAndExpiry(t *testing.T) {
	m := New(func(net.Conn) (uint32,error) { return 0,nil })
	var tokens []string
	for i:=0;i<maxSessionsPerPeer;i++ {
		token,_,err:=m.Start("198.51.100.2")
		if err!=nil {t.Fatal(err)}
		if len(token)!=39 {t.Fatalf("bad token length %d",len(token))}
		tokens=append(tokens,token)
	}
	if _,_,err:=m.Start("198.51.100.2");!errors.Is(err,ErrLimit){t.Fatal("missing per-peer quota")}
	if _,_,err:=m.Start("198.51.100.3");err!=nil{t.Fatal("unrelated peer rejected:",err)}
	if _,ok:=m.Read(tokens[0],"198.51.100.77");ok {t.Fatal("another peer accessed session")}
	conn,other:=net.Pipe()
	defer conn.Close()
	defer other.Close()
	if m.Begin(tokens[0],"198.51.100.77",conn)!=nil {t.Fatal("another peer started observation")}
	m.now=func()time.Time{return time.Now().Add(2*time.Minute)}
	if _,ok:=m.Read(tokens[0],"198.51.100.2");ok {t.Fatal("expired token remains usable")}
	if _,_,err:=m.Start("198.51.100.2");err!=nil {t.Fatal("expired token still uses quota:",err)}
}

func TestUnavailableAndFailedSample(t *testing.T) {
	m:=New(func(net.Conn)(uint32,error){return 0, errors.New("TCP_INFO unavailable")})
	token,_,err:=m.Start("192.0.2.1")
	if err!=nil {t.Fatal(err)}
	conn,other:=net.Pipe()
	defer conn.Close()
	defer other.Close()
	if done:=m.Begin(token,"192.0.2.1",conn);done!=nil {t.Fatal("unexpected valid TCP sample")}
	result,ok:=m.Read(token,"192.0.2.1")
	if !ok||result.Status!="unavailable"||result.FailedSamples!=1 {
		t.Fatalf("failed socket observation must be unavailable: %+v",result)
	}
}

func TestConcurrentRequestsAndSnapshots(t *testing.T) {
	var counter atomic.Uint32
	m:=New(func(net.Conn)(uint32,error){return counter.Load(),nil})
	token,_,err:=m.Start("192.0.2.1")
	if err!=nil {t.Fatal(err)}
	conn,other:=net.Pipe()
	defer conn.Close()
	defer other.Close()
	var wg sync.WaitGroup
	for i:=0;i<12;i++{
		wg.Add(1)
		go func(){
			defer wg.Done()
			end:=m.Begin(token,"192.0.2.1",conn)
			if end==nil {t.Error("could not observe concurrent request");return}
			counter.Add(1)
			end()
		}()
	}
	wg.Wait()
	result,ok:=m.Read(token,"192.0.2.1")
	if !ok||result.Status!="completed"||result.Connections!=1||result.CompletedRequests!=12 {
		t.Fatalf("bad concurrent summary %+v",result)
	}
}
