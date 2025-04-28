package main

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	pt "github.com/tannerklineintz/pytricia-go"
	"github.com/tidwall/redcon"
)

// TrieServer maintains one pytricia trie per logical DB (matching Redis’s
// integer‑indexed databases).
type TrieServer struct {
	dbs map[int]*pt.PyTricia
}

func NewTrieServer() *TrieServer {
	return &TrieServer{dbs: make(map[int]*pt.PyTricia)}
}

// getDB returns the trie for the given id, lazily creating it.
func (s *TrieServer) getDB(id int) *pt.PyTricia {
	tr, ok := s.dbs[id]
	if !ok {
		tr = pt.NewPyTricia()
		s.dbs[id] = tr
	}
	return tr
}

// currentDB looks up the database index stored in the connection context.
func currentDB(conn redcon.Conn) int {
	if ctx := conn.Context(); ctx != nil {
		if id, ok := ctx.(int); ok {
			return id
		}
	}
	return 0 // default DB 0, like Redis
}

// writeOK writes a simple string "+OK\r\n".
func writeOK(conn redcon.Conn) {
	conn.WriteString("+OK\r\n")
}

// HandleCommand implements the redcon handler signature.
func (s *TrieServer) HandleCommand(conn redcon.Conn, cmd redcon.Command) {
	if len(cmd.Args) == 0 {
		conn.WriteError("ERR empty command")
		return
	}
	name := strings.ToUpper(string(cmd.Args[0]))

	switch name {
	case "PING":
		conn.WriteString("+PONG\r\n")

	case "SELECT":
		if len(cmd.Args) != 2 {
			conn.WriteError("ERR wrong number of arguments for 'SELECT'")
			return
		}
		id, err := strconv.Atoi(string(cmd.Args[1]))
		if err != nil || id < 0 {
			conn.WriteError("ERR invalid DB index")
			return
		}
		conn.SetContext(id)
		writeOK(conn)

	case "SET":
		if len(cmd.Args) < 3 {
			conn.WriteError("ERR wrong number of arguments for 'SET'")
			return
		}
		cidr := string(cmd.Args[1])
		value := string(cmd.Args[2])
		db := s.getDB(currentDB(conn))
		if err := db.Insert(cidr, value); err != nil {
			conn.WriteError("ERR " + err.Error())
			return
		}
		writeOK(conn)

	case "GET":
		if len(cmd.Args) != 2 {
			conn.WriteError("ERR wrong number of arguments for 'GET'")
			return
		}
		key := string(cmd.Args[1])
		db := s.getDB(currentDB(conn))

		// First try exact match (CIDR key).
		if v := db.Get(key); v != nil {
			conn.WriteBulkString(fmt.Sprintf("%v", v))
			return
		} else {
			conn.WriteNull()
		}

	case "DEL":
		if len(cmd.Args) < 2 {
			conn.WriteError("ERR wrong number of arguments for 'DEL'")
			return
		}
		db := s.getDB(currentDB(conn))
		removed := 0
		for _, raw := range cmd.Args[1:] {
			cidr := string(raw)
			if err := db.Delete(cidr); err == nil {
				removed++
			}
		}
		conn.WriteInt(removed)

	case "DBSIZE":
		db := s.getDB(currentDB(conn))
		conn.WriteInt(len(db.Keys()))

	case "FLUSHDB":
		db := s.getDB(currentDB(conn))
		db.Clear()
		writeOK(conn)

	case "INFO":
		// If caller typed "INFO KEYSPACE" accept arg[1].
		if len(cmd.Args) > 2 {
			conn.WriteError("ERR wrong number of arguments for 'INFO'")
			return
		}
		subsection := "ALL"
		if len(cmd.Args) == 2 {
			subsection = strings.ToUpper(string(cmd.Args[1]))
		}

		if subsection == "KEYSPACE" || subsection == "ALL" {
			var b strings.Builder
			b.WriteString("# Keyspace\r\n")
			for id, trie := range s.dbs {
				fmt.Fprintf(&b, "db%d:keys=%d,expires=0,avg_ttl=0\r\n",
					id, len(trie.Keys()))
			}
			conn.WriteBulkString(b.String())
			return
		}

		// fall back to previous minimal INFO
		conn.WriteBulkString("# Triedis\r\n")

	default:
		conn.WriteError("ERR unknown command '" + name + "'")
	}
}

func main() {
	addr := flag.String("addr", "0.0.0.0:6379", "listen address")
	flag.Parse()

	srv := NewTrieServer()

	// Start the server. redcon will handle concurrency and RESP framing.
	log.Printf("Starting to serve requests on %v", *addr)
	err := redcon.ListenAndServe(*addr,
		srv.HandleCommand,
		func(conn redcon.Conn) bool { return true }, // accept all
		func(conn redcon.Conn, err error) {},        // on close
	)
	if err != nil {
		panic(err)
	}

	// Block forever. (redcon runs until fatal error or interrupt.)
	select {}
}
