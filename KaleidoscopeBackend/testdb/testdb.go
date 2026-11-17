//go:build integration

// Package testdb gives integration tests a throwaway database on the local
// MongoDB. It only builds with -tags integration.
package testdb

import (
	"bytes"
	"cmp"
	"context"
	"net"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoHost is the test MongoDB's address: KSCOPE_TEST_MONGO_HOST, or localhost:27017.
var mongoHost = cmp.Or(os.Getenv("KSCOPE_TEST_MONGO_HOST"), "localhost:27017")

// CreateDatabase connects with KSCOPE_TEST_MONGO_USER, KSCOPE_TEST_MONGO_PASSWORD
// and KSCOPE_TEST_MONGO_HOST (default localhost:27017) and returns a new, empty
// database that is dropped when the test ends. It fails the test when the user
// isn't set.
func CreateDatabase(t testing.TB) *mongo.Database {

	t.Helper()
	client := connectToTestMongo(t, mongoHost)

	//an ObjectID, not a timestamp: test binaries of several packages run at once
	db := client.Database("kscope_test_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		if err := db.Drop(context.Background()); err != nil {
			t.Errorf("dropping test database %s: %v", db.Name(), err)
		}
	})
	return db
}

// connectToTestMongo connects to host with the test credentials and pings it.
// The client is disconnected when the test ends.
func connectToTestMongo(t testing.TB, host string) *mongo.Client {

	t.Helper()
	user := os.Getenv("KSCOPE_TEST_MONGO_USER")
	if user == "" {
		t.Fatal("KSCOPE_TEST_MONGO_USER is not set; integration tests need the local MongoDB")
	}

	uri := (&url.URL{
		Scheme: "mongodb",
		User:   url.UserPassword(user, os.Getenv("KSCOPE_TEST_MONGO_PASSWORD")),
		Host:   host,
	}).String()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connecting to the test MongoDB: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(context.Background())
		t.Fatalf("reaching the test MongoDB at %s: %v", host, err)
	}

	t.Cleanup(func() { client.Disconnect(context.Background()) })
	return client
}

// ConnectWithReplyCut returns database reached through a local proxy, and a
// function that arms the proxy: the next request naming database is forwarded
// and applied, then its reply is dropped along with the connection, as when the
// server dies mid-write.
func ConnectWithReplyCut(t testing.TB, database string) (*mongo.Database, func()) {

	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting the reply-cut proxy: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	proxy := &replyCutProxy{listener: listener, target: mongoHost, database: []byte(database)}
	go proxy.serve()

	client := connectToTestMongo(t, listener.Addr().String())
	return client.Database(database), func() { proxy.armed.Store(true) }
}

// replyCutProxy relays connections to the test MongoDB; armed cuts the reply to
// the next request that names database.
type replyCutProxy struct {
	listener net.Listener
	target   string
	database []byte
	armed    atomic.Bool
}

// serve relays each accepted connection until the listener is closed.
func (p *replyCutProxy) serve() {

	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.relay(client)
	}
}

// relay copies client's traffic to and from a new connection to the target. A
// cut request is forwarded, and the first reply bytes after it are read and
// dropped before both connections close, so the server has applied it first.
func (p *replyCutProxy) relay(client net.Conn) {

	server, err := net.Dial("tcp", p.target)
	if err != nil {
		client.Close()
		return
	}

	var cut atomic.Bool
	go func() {

		defer client.Close()
		defer server.Close()
		buf := make([]byte, 64<<10)
		for {

			n, err := server.Read(buf)
			if n > 0 && cut.Load() {
				return
			}

			if n > 0 {
				if _, err := client.Write(buf[:n]); err != nil {
					return
				}
			}

			if err != nil {
				return
			}
		}
	}()

	defer client.Close()
	defer server.Close()
	buf := make([]byte, 64<<10)
	var tail []byte //the end of the previous read, so a name split across reads still matches
	for {

		n, err := client.Read(buf)
		if n > 0 {

			window := append(tail, buf[:n]...)
			if bytes.Contains(window, p.database) && p.armed.CompareAndSwap(true, false) {
				cut.Store(true) //before the request goes out, so its reply is the one dropped
			}

			if _, err := server.Write(buf[:n]); err != nil {
				return
			}

			if keep := len(p.database) - 1; len(window) > keep {
				window = window[len(window)-keep:]
			}
			tail = append([]byte(nil), window...)
		}

		if err != nil {
			return
		}
	}
}
