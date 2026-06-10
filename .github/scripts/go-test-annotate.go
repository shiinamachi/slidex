package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const maxTailLines = 80

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

type tailLog struct {
	lines []string
}

func (l *tailLog) add(s string) {
	if s == "" {
		return
	}
	parts := strings.SplitAfter(s, "\n")
	for _, part := range parts {
		if part == "" {
			continue
		}
		l.lines = append(l.lines, part)
		if len(l.lines) > maxTailLines {
			copy(l.lines, l.lines[len(l.lines)-maxTailLines:])
			l.lines = l.lines[:maxTailLines]
		}
	}
}

func (l tailLog) String() string {
	return strings.TrimSpace(strings.Join(l.lines, ""))
}

func main() {
	code, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

func run() (int, error) {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return 2, errors.New("usage: go-test-annotate -- [go test flags/packages]")
	}

	goArgs := append([]string{"test", "-json"}, args...)
	cmd := exec.Command("go", goArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
	}
	var stderr tailLog
	cmd.Stderr = io.MultiWriter(os.Stderr, tailWriter{log: &stderr})
	if err := cmd.Start(); err != nil {
		return 1, err
	}

	summary := newTestSummary()
	scanErr := summary.scan(stdout)
	waitErr := cmd.Wait()
	if scanErr != nil {
		emitAnnotation("go test output parse failed", scanErr.Error())
		return 1, scanErr
	}
	if waitErr == nil {
		return 0, nil
	}
	if !summary.annotated {
		message := firstNonEmptyForCI(summary.packageTail.String(), stderr.String(), waitErr.Error())
		emitAnnotation("go test failed", message)
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, waitErr
}

type tailWriter struct {
	log *tailLog
}

func (w tailWriter) Write(p []byte) (int, error) {
	w.log.add(string(p))
	return len(p), nil
}

type testSummary struct {
	testTails       map[string]*tailLog
	packageTails    map[string]*tailLog
	packageHadTests map[string]bool
	packageTail     tailLog
	annotated       bool
}

func newTestSummary() *testSummary {
	return &testSummary{
		testTails:       map[string]*tailLog{},
		packageTails:    map[string]*tailLog{},
		packageHadTests: map[string]bool{},
	}
}

func (s *testSummary) scan(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			fmt.Println(line)
			s.packageTail.add(line + "\n")
			continue
		}
		s.handle(ev)
	}
	return scanner.Err()
}

func (s *testSummary) handle(ev testEvent) {
	if ev.Output != "" {
		fmt.Print(ev.Output)
		s.tailForPackage(ev.Package).add(ev.Output)
		s.packageTail.add(ev.Output)
		if ev.Test != "" {
			s.tailForTest(ev.Package, ev.Test).add(ev.Output)
		}
		return
	}
	switch ev.Action {
	case "pass":
		if ev.Test == "" && ev.Package != "" {
			fmt.Printf("ok  \t%s\t%.3fs\n", ev.Package, ev.Elapsed)
		}
	case "fail":
		if ev.Test != "" {
			s.packageHadTests[ev.Package] = true
			title := fmt.Sprintf("go test failed: %s %s", ev.Package, ev.Test)
			emitAnnotation(title, firstNonEmptyForCI(s.tailForTest(ev.Package, ev.Test).String(), s.tailForPackage(ev.Package).String()))
			s.annotated = true
			return
		}
		fmt.Printf("FAIL\t%s\t%.3fs\n", ev.Package, ev.Elapsed)
		if ev.Package != "" && !s.packageHadTests[ev.Package] {
			emitAnnotation("go test package failed: "+ev.Package, s.tailForPackage(ev.Package).String())
			s.annotated = true
		}
	}
}

func (s *testSummary) tailForPackage(pkg string) *tailLog {
	if s.packageTails[pkg] == nil {
		s.packageTails[pkg] = &tailLog{}
	}
	return s.packageTails[pkg]
}

func (s *testSummary) tailForTest(pkg, test string) *tailLog {
	key := pkg + "\x00" + test
	if s.testTails[key] == nil {
		s.testTails[key] = &tailLog{}
	}
	return s.testTails[key]
}

func firstNonEmptyForCI(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "go test failed without diagnostic output"
}

func emitAnnotation(title, message string) {
	fmt.Printf("::error title=%s::%s\n", escapeWorkflowCommand(title), escapeWorkflowCommand(message))
}

func escapeWorkflowCommand(s string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	)
	return replacer.Replace(s)
}
