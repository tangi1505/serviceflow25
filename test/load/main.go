// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Command load is the performance tool of itsm (Hauptplan H11, E32, E50, E51, E63 to E65, E69).
//
//	go run ./test/load machine
//	go run ./test/load generate --size=ci --out=/tmp/data
//	go run ./test/load run --stage=quick --target=http://127.0.0.1:18080 \
//	    --baseline-target=http://127.0.0.1:18081 --out=/tmp/quick.json
//	go run ./test/load gate --stage=quick --result=/tmp/quick.json
//	go run ./test/load run --stage=full --target=http://127.0.0.1:18080 --out=/tmp/full.json
//	go run ./test/load gate --stage=full --result=/tmp/full.json
//	go run ./test/load baseline --result=/tmp/quick.json --binary=/tmp/itsm --note="why"
//
// The quick stage measures the new build and the approved baseline build alternately
// (A B A B A B) on the same machine, so that load from elsewhere hits both alike (E69). The
// baseline build is the binary that "baseline" stored below --store.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"group24.de/itsm/test/load/internal/gate"
	"group24.de/itsm/test/load/internal/gen"
	"group24.de/itsm/test/load/internal/machine"
	"group24.de/itsm/test/load/internal/measure"
	"group24.de/itsm/test/load/internal/scenario"
)

// Rules of the quick stage, applied to the best of the repeated runs (E76). FloorMs absorbs
// measurement noise on very fast endpoints: five quick runs on the measuring machine on
// 2026-10-20 varied by at most 0.04 ms in p95 (Hauptplan H11, Umsetzungsstand); 0.1 ms leaves a
// margin of more than two.
var quickRules = gate.Rules{MaxSlowdown: 0.10, FloorMs: 0.1}

// DefaultStore is where the baseline binary lives (a Docker volume in the tool container).
const DefaultStore = "/var/lib/itsm-load"

// BaselineBinary is the file name of the stored baseline build.
const BaselineBinary = "itsm-baseline"

// ExitBusy is the exit code when the load guard refuses to measure (E69).
const ExitBusy = 3

type stage struct {
	workers          int
	warmup, duration time.Duration
	repeat           int
	// bulkRows is the size of a throughput run (LZ5).
	bulkRows int
}

var stages = map[string]stage{
	"quick": {workers: 8, warmup: time.Second, duration: 4 * time.Second, repeat: 4, bulkRows: 20000},
	"full":  {workers: 32, warmup: 10 * time.Second, duration: 60 * time.Second, repeat: 3, bulkRows: 100000},
}

// errBusy marks a refused measurement.
var errBusy = errors.New("rechner ausgelastet")

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		if errors.Is(err, errBusy) {
			os.Exit(ExitBusy)
		}
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("command missing: machine, generate, run, gate, baseline")
	}
	switch args[0] {
	case "machine":
		m := machine.Detect()
		load, _ := loadAverage()
		return writeJSON(out, struct {
			machine.Machine
			Load1 float64 `json:"load1"`
		}{m, load})
	case "generate":
		return cmdGenerate(args[1:], out)
	case "run":
		return cmdRun(args[1:], out)
	case "gate":
		return cmdGate(args[1:], out)
	case "baseline":
		return cmdBaseline(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func cmdGenerate(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	size := fs.String("size", "ci", "tiny, ci or prod")
	dir := fs.String("out", "", "output directory")
	seed := fs.Uint64("seed", 1, "seed; same seed, same data")
	only := fs.String("only", "", "comma-separated kinds, default all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, ok := gen.Sizes[*size]
	if !ok || *dir == "" {
		return errors.New("generate needs --size=tiny|ci|prod and --out=DIR")
	}
	if err := os.MkdirAll(*dir, 0o750); err != nil {
		return err
	}
	kinds := gen.Kinds
	if *only != "" {
		kinds = strings.Split(*only, ",")
	}
	for _, kind := range kinds {
		start := time.Now()
		path := filepath.Join(*dir, kind+".ndjson")
		f, err := os.Create(path) //nolint:gosec // G304: the operator chooses the output directory
		if err != nil {
			return err
		}
		n, err := gen.Write(f, kind, s, *seed)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", kind, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%-12s %10d Zeilen %8.1f MB %6.1f s  %s\n", kind, n,
			float64(info.Size())/1e6, time.Since(start).Seconds(), path)
	}
	return nil
}

func cmdRun(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	stageName := fs.String("stage", "quick", "quick or full")
	target := fs.String("target", "", "base URL of the new build, e.g. http://127.0.0.1:18080")
	baseTarget := fs.String("baseline-target", "", "base URL of the baseline build (quick stage, A/B)")
	baseSHA := fs.String("baseline-sha256", "", "SHA-256 of the baseline binary, recorded in the report")
	file := fs.String("out", "", "write the report as JSON to this file")
	delay := fs.Duration("inject-delay", 0, "put a proxy with this extra delay in front of the new build (self-test of the gate)")
	ignoreLoad := fs.Bool("ignore-load", false, "measure even when the machine is busy")
	waitIdle := fs.Duration("wait-for-idle", 0, "wait up to this long for the load to drop below the limit before measuring")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, ok := stages[*stageName]
	if !ok || *target == "" {
		return errors.New("run needs --stage=quick|full and --target=URL")
	}
	if v := os.Getenv("ITSM_LOAD_INJECT_DELAY"); v != "" && *delay == 0 {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("ITSM_LOAD_INJECT_DELAY: %w", err)
		}
		*delay = d
	}
	if os.Getenv("ITSM_LOAD_IGNORE_LOAD") == "1" {
		*ignoreLoad = true
	}

	// Load guard (E69): a busy machine gives no verdict, neither pass nor fail. The 1-minute
	// load average decays slowly after the test phase of the CI, so the guard may wait first.
	load, err := loadAverage()
	if err == nil {
		limit := float64(runtime.NumCPU()) / 2
		deadline := time.Now().Add(*waitIdle)
		for load > limit && !*ignoreLoad && time.Now().Before(deadline) {
			fmt.Fprintf(out, "Last %.1f auf %d Kernen, warte bis unter %.1f …\n", load, runtime.NumCPU(), limit)
			time.Sleep(10 * time.Second)
			if load, err = loadAverage(); err != nil {
				break
			}
		}
		if load > limit && !*ignoreLoad {
			return fmt.Errorf("%w: Last %.1f auf %d Kernen, erlaubt sind %.1f; Messung ungültig, bitte wiederholen, wenn der Rechner frei ist (oder --ignore-load)",
				errBusy, load, runtime.NumCPU(), limit)
		}
		fmt.Fprintf(out, "Last beim Start: %.2f auf %d Kernen\n", load, runtime.NumCPU())
	}

	newBase := strings.TrimRight(*target, "/")
	if *delay > 0 {
		proxied, stop, err := slowProxy(newBase, *delay)
		if err != nil {
			return err
		}
		defer stop()
		fmt.Fprintf(out, "Achtung: Proxy mit %s Zusatzverzögerung vor %s (Selbsttest des Gates)\n", *delay, newBase)
		newBase = proxied
	}
	oldBase := strings.TrimRight(*baseTarget, "/")

	ctx := context.Background()
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{MaxIdleConnsPerHost: st.workers * 2},
	}
	rep := gate.Report{Stage: *stageName, Machine: machine.Detect(), Load: load,
		Results: map[string]measure.Result{}, Waiting: map[string]string{}, BaselineSHA256: *baseSHA}
	if oldBase != "" {
		rep.Baseline = map[string]measure.Result{}
	}
	opts := measure.Options{Workers: st.workers, Warmup: st.warmup, Duration: st.duration}
	for _, s := range scenario.All() {
		if !s.Ready() {
			rep.Waiting[s.ID] = s.WaitsFor
			continue
		}
		if s.Throughput != nil {
			// A run as a whole (LZ5), repeated like the other scenarios with the builds taking
			// turns (A B, B A, …); the best run of each counts (E76). A single run each was
			// noisy: the same build against itself differed by 12 % (WF3).
			var newRuns, oldRuns []measure.Result
			oldFailed := ""
			for i := range st.repeat {
				order := []bool{true, false}
				if i%2 == 1 {
					order = []bool{false, true}
				}
				for _, isNew := range order {
					switch {
					case isNew:
						m, err := throughput(ctx, client, s, newBase, st.bulkRows)
						if err != nil {
							return fmt.Errorf("%s am neuen Stand: %w", s.ID, err)
						}
						newRuns = append(newRuns, m)
					case oldBase != "" && oldFailed == "":
						b, err := throughput(ctx, client, s, oldBase, st.bulkRows)
						if err != nil {
							oldFailed = err.Error()
							continue
						}
						oldRuns = append(oldRuns, b)
					}
				}
			}
			m := bestThroughput(newRuns)
			rep.Results[s.ID] = m
			fmt.Fprintf(out, "%-5s neu        %d Zeilen, %8.0f Zeilen/s  (Läufe: %s)\n", s.ID, m.Rows, m.RPS, rates(newRuns))
			switch {
			case oldFailed != "":
				rep.Baseline[s.ID] = measure.Result{Unavailable: oldFailed}
				fmt.Fprintf(out, "%-5s Basislinie nicht messbar: %s\n", s.ID, oldFailed)
			case oldBase != "":
				b := bestThroughput(oldRuns)
				rep.Baseline[s.ID] = b
				fmt.Fprintf(out, "%-5s Basislinie %d Zeilen, %8.0f Zeilen/s  (Läufe: %s)\n", s.ID, b.Rows, b.RPS, rates(oldRuns))
			}
			continue
		}
		newReq, err := prepare(ctx, client, s, newBase)
		if err != nil {
			return fmt.Errorf("%s: vorbereiten am neuen Stand: %w", s.ID, err)
		}
		var oldReq measure.RequestFunc
		var unavailable string
		if oldBase != "" {
			// A baseline build that cannot serve a new scenario (it lacks the endpoint) is not
			// measured at all: the result says so, and the gate reports instead of comparing.
			oldReq, err = prepare(ctx, client, s, oldBase)
			if err != nil {
				unavailable = err.Error()
				oldReq = nil
			}
		}
		var newRuns, oldRuns []measure.Result
		for i := range st.repeat {
			// Alternate, and swap the order every repetition (A B, B A, A B …), so that drift of
			// the machine and warm caches hit both builds alike.
			if oldReq == nil || i%2 == 0 {
				newRuns = append(newRuns, measure.Run(ctx, client, newReq, opts))
				if oldReq != nil {
					oldRuns = append(oldRuns, measure.Run(ctx, client, oldReq, opts))
				}
			} else {
				oldRuns = append(oldRuns, measure.Run(ctx, client, oldReq, opts))
				newRuns = append(newRuns, measure.Run(ctx, client, newReq, opts))
			}
		}
		m := measure.Best(newRuns)
		rep.Results[s.ID] = m
		fmt.Fprintf(out, "%-5s neu        p50 %7.2f ms  p95 %7.2f ms  p99 %7.2f ms  %8.0f/s  Fehler %d  (p95 der Läufe: %s)\n",
			s.ID, m.P50ms, m.P95ms, m.P99ms, m.RPS, m.Errors, p95s(newRuns))
		switch {
		case unavailable != "":
			rep.Baseline[s.ID] = measure.Result{Unavailable: unavailable}
			fmt.Fprintf(out, "%-5s Basislinie nicht messbar: %s\n", s.ID, unavailable)
		case oldBase != "":
			b := measure.Best(oldRuns)
			rep.Baseline[s.ID] = b
			fmt.Fprintf(out, "%-5s Basislinie p50 %7.2f ms  p95 %7.2f ms  p99 %7.2f ms  %8.0f/s  Fehler %d  (p95 der Läufe: %s)\n",
				s.ID, b.P50ms, b.P95ms, b.P99ms, b.RPS, b.Errors, p95s(oldRuns))
		}
	}
	for _, s := range scenario.All() {
		if w, ok := rep.Waiting[s.ID]; ok {
			fmt.Fprintf(out, "%-5s noch nicht messbar, wartet auf %s\n", s.ID, w)
		}
	}
	if *file != "" {
		return writeFile(*file, rep)
	}
	return nil
}

// bestThroughput is the run with the most rows per second.
func bestThroughput(runs []measure.Result) measure.Result {
	best := runs[0]
	for _, r := range runs[1:] {
		if r.RPS > best.RPS {
			best = r
		}
	}
	return best
}

// rates lists the rows per second of the runs, for the log.
func rates(runs []measure.Result) string {
	parts := make([]string, len(runs))
	for i, r := range runs {
		parts[i] = fmt.Sprintf("%.0f", r.RPS)
	}
	return strings.Join(parts, "/")
}

// throughput measures one run of a throughput scenario: rows per second until it is done.
func throughput(ctx context.Context, client *http.Client, s scenario.Scenario, base string, rows int) (measure.Result, error) {
	start := time.Now()
	n, err := s.Throughput(ctx, client, base, rows)
	if err != nil {
		return measure.Result{}, err
	}
	took := time.Since(start)
	return measure.Result{Requests: 1, Rows: n, RPS: float64(n) / took.Seconds()}, nil
}

// prepare binds a scenario to a target: runs its Prepare step, if any, and returns the
// request builder for the measurement.
func prepare(ctx context.Context, client *http.Client, s scenario.Scenario, base string) (measure.RequestFunc, error) {
	rf := s.Request
	if s.Prepare != nil {
		var err error
		if rf, err = s.Prepare(ctx, client, base); err != nil {
			return nil, err
		}
	}
	return func(ctx context.Context) (*http.Request, error) { return rf(ctx, base) }, nil
}

func cmdGate(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	stageName := fs.String("stage", "quick", "quick or full")
	resultFile := fs.String("result", "", "report of the run")
	baselineFile := fs.String("baseline", "test/load/baseline.json", "approval record of the baseline (quick stage)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var cur gate.Report
	if err := readFile(*resultFile, &cur); err != nil {
		return err
	}
	var findings []gate.Finding
	switch *stageName {
	case "quick":
		var base gate.Report
		switch err := readFile(*baselineFile, &base); {
		case err != nil:
			fmt.Fprintf(out, "Hinweis: %s nicht lesbar (%v)\n", *baselineFile, err)
		case cur.BaselineSHA256 != "" && base.BinarySHA256 != cur.BaselineSHA256:
			fmt.Fprintln(out, "Hinweis: Das gemessene Basis-Binary ist nicht das aus", *baselineFile)
		case base.ApprovedBy == "":
			fmt.Fprintln(out, "Hinweis: Die Basislinie ist noch nicht freigegeben (approved_by ist leer).")
		}
		findings = gate.Quick(cur, quickRules, scenario.All())
	case "full":
		findings = gate.Full(cur, scenario.All())
	default:
		return fmt.Errorf("unknown stage %q", *stageName)
	}
	for _, f := range findings {
		mark := "ok  "
		if f.Fail {
			mark = "FAIL"
		}
		fmt.Fprintf(out, "%s %-5s %s\n", mark, f.ID, f.Text)
	}
	if gate.Failed(findings) {
		return errors.New("leistungsgate nicht bestanden")
	}
	fmt.Fprintln(out, "Leistungsgate bestanden")
	return nil
}

// cmdBaseline stores the binary of a run as the baseline build and writes the approval record.
func cmdBaseline(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("baseline", flag.ContinueOnError)
	resultFile := fs.String("result", "", "report of a quick run of this binary")
	binary := fs.String("binary", "", "the itsm binary that was measured")
	store := fs.String("store", DefaultStore, "directory that keeps the baseline binary")
	file := fs.String("out", "test/load/baseline.json", "approval record")
	note := fs.String("note", "", "why the baseline changes (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *note == "" {
		return errors.New("baseline needs --note: a new baseline must say why")
	}
	if *binary == "" {
		return errors.New("baseline needs --binary: the measured itsm binary")
	}
	var rep gate.Report
	if err := readFile(*resultFile, &rep); err != nil {
		return err
	}
	sum, err := copyBinary(*binary, filepath.Join(*store, BaselineBinary))
	if err != nil {
		return err
	}
	rep.Baseline, rep.BaselineSHA256 = nil, ""
	rep.BinarySHA256 = sum
	rep.Note = *note
	rep.ApprovedBy = "" // only the review sets this, by hand (Hauptplan E64)
	if err := writeFile(*file, rep); err != nil {
		return err
	}
	fmt.Fprintf(out, "Basislinie geschrieben: %s, Binary %s (sha256 %s…); wartet auf Freigabe\n",
		*file, filepath.Join(*store, BaselineBinary), sum[:12])
	return nil
}

func copyBinary(src, dst string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return "", err
	}
	in, err := os.Open(src) //nolint:gosec // G304: the operator names the binary
	if err != nil {
		return "", err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	o, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o750) //nolint:gosec // G302: an executable
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(o, h), in); err != nil {
		_ = o.Close()
		return "", err
	}
	if err := o.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the operator names the file
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// loadAverage reads the 1-minute load average from /proc.
func loadAverage() (float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, errors.New("loadavg: empty")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// slowProxy starts a reverse proxy on a free local port that delays every request.
func slowProxy(target string, delay time.Duration) (string, func(), error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		rp.ServeHTTP(w, r)
	})
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return "http://" + ln.Addr().String(), func() { _ = srv.Close() }, nil
}

func p95s(rs []measure.Result) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("%.2f", r.P95ms)
	}
	return strings.Join(parts, "/")
}

func readFile(path string, v any) error {
	if path == "" {
		return errors.New("file missing")
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304: the operator chooses the file
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
