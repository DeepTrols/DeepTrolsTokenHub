// Command perfbench runs gateway load tests against an OpenAI-compatible
// endpoint using the perfbench package (ported from TokenHub, Apache-2.0).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/deeptrols/api/tools/perfbench"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("a command is required")
	}
	switch args[0] {
	case "mocker":
		return runMocker(args[1:])
	case "run":
		return runBenchmark(ctx, args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: perfbench <mocker|run> [flags]")
}

func runMocker(args []string) error {
	flags := flag.NewFlagSet("mocker", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:18081", "listen address")
	latency := flags.Duration("latency", 5*time.Millisecond, "response latency")
	responseBytes := flags.Int("response-bytes", 1024, "approximate response bytes")
	streamChunks := flags.Int("stream-chunks", 8, "stream content chunks")
	chunkInterval := flags.Duration("chunk-interval", time.Millisecond, "delay between stream chunks")
	failureEvery := flags.Int("failure-every", 0, "return a failure every N requests (zero disables)")
	failureStatus := flags.Int("failure-status", http.StatusServiceUnavailable, "injected HTTP failure status")
	if err := flags.Parse(args); err != nil {
		return err
	}
	handler := perfbench.NewMockHandler(perfbench.MockConfig{
		Latency: *latency, ResponseBytes: *responseBytes,
		StreamChunks: *streamChunks, ChunkInterval: *chunkInterval,
		FailureEvery: uint64(*failureEvery), FailureStatus: *failureStatus,
	})
	fmt.Fprintf(os.Stderr, "deterministic upstream listening on http://%s\n", *listen)
	return (&http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe()
}

func runBenchmark(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	baseURL := flags.String("base-url", "", "gateway base URL (required)")
	apiKey := flags.String("api-key", "", "gateway API key")
	model := flags.String("model", "deepseek-chat", "model name")
	protocol := flags.String("protocol", "chat", "chat|embeddings")
	stream := flags.Bool("stream", false, "streaming requests")
	mode := flags.String("mode", "concurrency", "concurrency|rate")
	concurrency := flags.Int("concurrency", 20, "concurrent workers")
	rate := flags.Int("rate", 0, "fixed rate (rps) when mode=rate")
	duration := flags.Duration("duration", 30*time.Second, "measurement duration")
	warmup := flags.Duration("warmup", 0, "warmup duration")
	timeout := flags.Duration("timeout", 30*time.Second, "per-request timeout")
	requestBytes := flags.Int("request-bytes", 256, "request payload size")
	expectedLatency := flags.Duration("expected-upstream-latency", 0, "expected upstream latency")
	out := flags.String("out", "md", "output format: md|json")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *baseURL == "" {
		return errors.New("--base-url is required")
	}

	loadMode := perfbench.ModeConcurrency
	if *mode == "rate" {
		loadMode = perfbench.ModeRate
	}
	benchProtocol := perfbench.ProtocolChat
	if *protocol == "embeddings" {
		benchProtocol = perfbench.ProtocolEmbedding
	}

	result, err := perfbench.Run(ctx, perfbench.Config{
		Label:                   "deep-trols-gateway",
		BaseURL:                 *baseURL,
		APIKey:                  *apiKey,
		Model:                   *model,
		Protocol:                benchProtocol,
		Stream:                  *stream,
		Mode:                    loadMode,
		Concurrency:             *concurrency,
		Rate:                    *rate,
		Duration:                *duration,
		Warmup:                  *warmup,
		Timeout:                 *timeout,
		RequestBytes:            *requestBytes,
		ExpectedUpstreamLatency: *expectedLatency,
	})
	if err != nil {
		return err
	}
	if *out == "json" {
		return perfbench.WriteJSON(os.Stdout, result)
	}
	fmt.Fprint(os.Stdout, perfbench.Markdown(result))
	return nil
}
