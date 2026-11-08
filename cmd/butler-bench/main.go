// butler-bench measures fixed synthetic tasks through Butler's public ingress
// and observes the product ledger. It is an operator command, outside the agent.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := command(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "butler-bench:", err)
		os.Exit(1)
	}
}
func command(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("choose list, system, run, report, replay or compare (see cmd/butler-bench/README.md)")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	out := fs.String("out", "", "new output file, stdout by default; run requires a new directory")
	switch args[0] {
	case "system":
		if err := parseFlags(fs, args); err != nil {
			return err
		}
		return system(ctx, ".", *out)
	case "report", "replay", "compare":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 && args[0] != "compare" || fs.NArg() != 2 && args[0] == "compare" {
			return errors.New("pass one run directory for report or two for compare")
		}
		a, ar, err := readRun(fs.Arg(0))
		if err != nil {
			return err
		}
		if args[0] == "report" {
			return writeJSON(*out, summarize(a, ar))
		}
		if args[0] == "replay" {
			result, err := replay(fs.Arg(0), a, ar)
			if err != nil {
				return err
			}
			return writeJSON(*out, result)
		}
		b, br, err := readRun(fs.Arg(1))
		if err != nil {
			return err
		}
		report, err := compare(a, ar, b, br)
		if err != nil {
			return err
		}
		return writeJSON(*out, report)
	case "run", "list":
		group := fs.String("group", "", "all, discovery, reasoning, execution, workflow or recipient")
		ids := fs.String("cases", "", "comma-separated case IDs")
		seed := fs.String("seed", "butler-v1", "shared deterministic seed")
		repeats := fs.Int("repeat", 1, "repetitions per case")
		timeout := fs.Duration("timeout", 4*time.Minute, "deadline for one whole case")
		setupFile := fs.String("setup", "", "common deployed revision and fixture manifest")
		endpoint := fs.String("endpoint", "", "isolated Butler Core origin")
		token := fs.String("owner-token-file", "", "synthetic lab owner's token file")
		traceDir := fs.String("trace-dir", "", "private directory shared with the lab Core via BUTLER_BENCH_TRACE_DIR")
		provider := fs.String("provider", "", "expected execution ledger provider")
		model := fs.String("model", "", "expected execution ledger model")
		effort := fs.String("effort", "", "required reasoning effort observed in each native response")
		if err := parseFlags(fs, args); err != nil {
			return err
		}
		var setup Setup
		if *setupFile != "" {
			raw, err := os.ReadFile(*setupFile)
			if err != nil {
				return err
			}
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if !json.Valid(raw) || decoder.Decode(&setup) != nil || !commitRE.MatchString(setup.Commit) || !shaRE.MatchString(setup.Environment) {
				return errors.New("setup needs the deployed 40-digit commit and a 64-digit SHA-256 of the shared lab configuration")
			}
			for _, v := range []string{setup.Site.Origin, setup.Site.SPAURL} {
				if v == "" {
					continue
				}
				u, err := url.Parse(v)
				if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
					return errors.New("recipient must be an operator-owned HTTPS URL")
				}
			}
			if setup.Site.Origin != "" {
				u, _ := url.Parse(setup.Site.Origin)
				if u.Path != "" || u.RawQuery != "" || u.ForceQuery {
					return errors.New("browser recipient origin must have no path or query")
				}
			}
		}
		if args[0] == "list" && *group == "" && *ids == "" {
			*group = "all"
		}
		p, work, err := makePlan(setup, *seed, *group, *ids, *repeats, *timeout)
		if err != nil {
			return err
		}
		if args[0] == "list" {
			type entry struct {
				ID       string   `json:"id"`
				Group    string   `json:"group"`
				Feature  string   `json:"feature"`
				Tasks    []string `json:"user_messages"`
				Requires string   `json:"missing_fixture,omitempty"`
			}
			var list []entry
			for i, c := range work {
				if p.Slots[i].Repeat == 1 {
					e := entry{ID: c.ID, Group: c.Group, Feature: c.Feature, Requires: c.Missing}
					for _, s := range c.Steps {
						e.Tasks = append(e.Tasks, s.Prompt)
					}
					list = append(list, e)
				}
			}
			return writeJSON(*out, list)
		}
		if *setupFile == "" || *model == "" || *provider == "" || *token == "" || *out == "" || *out == "-" || *traceDir == "" || *endpoint == "" {
			return errors.New("run needs -setup, -provider, -model, -endpoint, -owner-token-file, -trace-dir and -out")
		}
		candidate := Candidate{Provider: *provider, Model: *model, Effort: *effort}
		if !candidate.validEffort() {
			return errors.New("unsupported reasoning effort profile")
		}
		preflight, cancel := context.WithTimeout(ctx, 8*time.Second)
		client, err := newClient(preflight, *endpoint, *token, candidate)
		cancel()
		if err != nil {
			return err
		}
		defer client.db.Close()
		defer client.http.CloseIdleConnections()
		client.traceDir, err = filepath.Abs(*traceDir)
		if err != nil {
			return err
		}
		info, err := os.Stat(client.traceDir)
		if err != nil || !info.IsDir() {
			return errors.New("create the shared private trace directory before running")
		}
		return execute(ctx, client, *out, newRun(p, candidate, setup), work)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}
