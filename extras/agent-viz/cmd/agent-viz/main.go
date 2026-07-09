// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/pdlc-os/fabric/extras/agent-viz/internal/logparser"
	"github.com/pdlc-os/fabric/extras/agent-viz/internal/playback"
	"github.com/pdlc-os/fabric/extras/agent-viz/internal/server"
)

func main() {
	logFile := flag.String("log-file", "", "Path to GCP log JSON export file")
	fsLog := flag.String("fs-log", "", "Path to fs-watcher NDJSON log (replaces file events from primary log)")
	maxDepth := flag.Int("max-depth", 3, "Maximum directory depth for file graph nodes (0 = unlimited)")
	port := flag.Int("port", 8080, "Port to serve on")
	devMode := flag.Bool("dev", false, "Serve web assets from disk (development mode)")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	flag.Parse()

	if *logFile == "" {
		fmt.Fprintln(os.Stderr, "Usage: agent-viz --log-file /path/to/logs.json [--fs-log /path/to/fs.ndjson] [--port 8080]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("Parsing log file: %s", *logFile)
	result, err := logparser.ParseLogFile(*logFile, *fsLog, *maxDepth)
	if err != nil {
		log.Fatalf("Error parsing log file: %v", err)
	}

	log.Printf("Found %d agents, %d files, %d events",
		len(result.Manifest.Agents),
		len(result.Manifest.Files),
		len(result.Events))

	for _, a := range result.Manifest.Agents {
		idShort := a.ID
		if len(idShort) > 8 {
			idShort = idShort[:8]
		}
		log.Printf("  Agent: %s (id=%s, harness=%s)", a.Name, idShort, a.Harness)
	}
	if len(result.Manifest.Files) > 0 {
		for _, f := range result.Manifest.Files {
			log.Printf("  File: %s (dir=%v)", f.ID, f.IsDir)
		}
	} else {
		log.Printf("  (no files detected from tool calls — files will appear dynamically during playback)")
	}

	engine, err := playback.NewEngine(result)
	if err != nil {
		log.Fatalf("Error creating playback engine: %v", err)
	}
	defer engine.Close()

	srv := server.New(engine)

	if !*noBrowser {
		go openBrowser(fmt.Sprintf("http://localhost:%d", *port))
	}

	if err := srv.Start(*port, *devMode); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
