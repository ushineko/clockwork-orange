// modelget: spike prototype for a model-acquisition frontend.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

type File struct {
	Name   string
	URL    string
	Size   int64
	SHA256 string
}

type Model struct {
	Name, Description, License string
	Gated                      bool
	Files                      []File
}

var catalog = []Model{
	{Name: "taesd", Description: "Tiny autoencoder for fast previews", License: "MIT", Files: []File{{
		Name: "taesd_decoder.safetensors", Size: 4895612,
		URL:    "https://huggingface.co/madebyollin/taesd/resolve/main/taesd_decoder.safetensors",
		SHA256: "f0fb51dd10d41c26612c070fa0b52ea0215a5ff90792134b4971109dd713c019"}}},
	{Name: "sd15-q8", Description: "Stable Diffusion 1.5, Q8_0 GGUF", License: "CreativeML OpenRAIL-M", Files: []File{{
		Name: "stable-diffusion-v1-5-pruned-emaonly-Q8_0.gguf", Size: 1763578176,
		URL:    "https://huggingface.co/second-state/stable-diffusion-v1-5-GGUF/resolve/main/stable-diffusion-v1-5-pruned-emaonly-Q8_0.gguf",
		SHA256: "d0555243938c62faeefb4ac93f6c7a053ad373a4290c5256bce229aeb193bf94"}}},
}

// Progress is the event a GUI would render.
type Progress struct {
	File        string
	Done, Total int64
	Resumed     int64 // bytes already on disk when the transfer started
}

var ErrChecksum = errors.New("checksum mismatch")

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
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

// Download fetches f into dir, resuming <name>.part with an HTTP Range request,
// verifies the SHA-256, then renames into place. Cancellable via ctx.
func Download(ctx context.Context, dir string, f File, ev chan<- Progress) error {
	final := filepath.Join(dir, f.Name)
	part := final + ".part"
	if sum, err := hashFile(final); err == nil && sum == f.SHA256 {
		return nil // already present and valid
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flag := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flag |= os.O_APPEND
	case http.StatusOK:
		flag |= os.O_TRUNC // server ignored Range: restart
		have = 0
	case http.StatusRequestedRangeNotSatisfiable:
		have = 0
		_ = os.Remove(part)
		return fmt.Errorf("stale partial file removed, retry")
	default:
		return fmt.Errorf("http %s", resp.Status)
	}
	out, err := os.OpenFile(part, flag, 0o644)
	if err != nil {
		return err
	}
	resumed := have
	buf := make([]byte, 256<<10)
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				return err
			}
			have += int64(n)
			if ev != nil && time.Since(last) > 100*time.Millisecond {
				ev <- Progress{f.Name, have, f.Size, resumed}
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr // .part kept for resume
		}
	}
	out.Close()
	if ev != nil {
		ev <- Progress{f.Name, have, f.Size, resumed}
	}
	sum, err := hashFile(part)
	if err != nil {
		return err
	}
	if sum != f.SHA256 {
		_ = os.Remove(part)
		return fmt.Errorf("%w: got %s want %s", ErrChecksum, sum[:12], f.SHA256[:12])
	}
	return os.Rename(part, final)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: modelget list | pull [-dir D] [-badsum] <model>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "list":
		fmt.Printf("%-10s %-10s %-24s %s\n", "NAME", "SIZE", "LICENSE", "DESCRIPTION")
		for _, m := range catalog {
			var t int64
			for _, f := range m.Files {
				t += f.Size
			}
			fmt.Printf("%-10s %-10s %-24s %s\n", m.Name, fmt.Sprintf("%.1f MB", float64(t)/1e6), m.License, m.Description)
		}
	case "pull":
		dir, bad, name := ".", false, ""
		for i := 2; i < len(os.Args); i++ {
			switch a := os.Args[i]; {
			case a == "-dir":
				i++
				dir = os.Args[i]
			case a == "-badsum":
				bad = true
			default:
				name = a
			}
		}
		var m *Model
		for i := range catalog {
			if catalog[i].Name == name {
				m = &catalog[i]
			}
		}
		if m == nil {
			fmt.Println("unknown model", name)
			os.Exit(2)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		for _, f := range m.Files {
			if bad {
				f.SHA256 = strings.Repeat("0", 64)
			}
			ev := make(chan Progress, 8)
			done := make(chan struct{})
			go func() {
				for p := range ev {
					fmt.Printf("\r%s %5.1f%% %d/%d MB (resumed from %d MB)", p.File, 100*float64(p.Done)/float64(p.Total), p.Done>>20, p.Total>>20, p.Resumed>>20)
				}
				close(done)
			}()
			err := Download(ctx, filepath.Join(dir, m.Name), f, ev)
			close(ev)
			<-done
			fmt.Println()
			if err != nil {
				fmt.Println("ERROR:", err)
				os.Exit(1)
			}
		}
		fmt.Println("ok")
	}
}
