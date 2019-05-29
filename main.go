package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lib/pq"
)

func main() {
	mux := http.NewServeMux()
	pqconnector, err := pq.NewConnector("sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	mux.Handle("/ws", &server{db: sql.OpenDB(pqconnector)})
	mux.Handle("/", http.FileServer(http.Dir("static")))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}

}

type server struct {
	db *sql.DB
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := s.serveHTTP(w, r); err != nil {
		log.Print(err)
	}
}

func (s *server) serveHTTP(w http.ResponseWriter, r *http.Request) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	return s.wsloop(r.Context(), conn)
}

func (s *server) wsloop(ctx context.Context, conn *websocket.Conn) error {
	for {
		var q struct{ Query string }
		if err := conn.ReadJSON(&q); err != nil {
			return err
		}

		if err := conn.WriteJSON(s.do(ctx, q.Query)); err != nil {
			return err
		}
	}
}

type result struct {
	Columns  []string        `json:"columns,omitempty"`
	Rows     [][]interface{} `json:"rows,omitempty"`
	Duration string          `json:"duration,omitempty"`
	Err      string          `json:"err,omitempty"`
	Stats    *sql.DBStats    `json:"stats,omitempty"`
}

func (s *server) do(ctx context.Context, query string) result {
	start := time.Now()
	if query == "stats" {
		stats := s.db.Stats()
		return result{Stats: &stats}
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return result{Err: err.Error()}
	}
	columns, err := rows.Columns()
	if err != nil {
		return result{Err: err.Error()}
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return result{Err: err.Error()}
	}
	var scannedRows [][]interface{}
	for rows.Next() {
		var scanvalues []interface{}
		for i := range types {
			scanvalues = append(scanvalues,
				reflect.New(types[i].ScanType()).Interface())
		}
		if err := rows.Scan(scanvalues...); err != nil {
			return result{Err: err.Error()}
		}
		scannedRows = append(scannedRows,
			scanvalues)
	}
	if err := rows.Err(); err != nil {
		return result{Err: err.Error()}
	}
	return result{
		Columns:  columns,
		Rows:     scannedRows,
		Duration: time.Since(start).String(),
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}
