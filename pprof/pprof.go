package pprof

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
)

type PprofServer uint32

func (p PprofServer) Start() {
	addr := fmt.Sprintf("0.0.0.0:%d", p)
	fmt.Printf("Start pprof server, listen addr %s/debug/pprof\n", addr)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func (PprofServer) Stop() {
	fmt.Printf("Stop pprof server\n")
}
