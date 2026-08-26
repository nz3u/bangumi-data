// Package download 获取 bangumi/Archive 最新导出快照的元信息并多线程下载。
//
// aux/latest.json 描述最新 release asset 的名称、大小、下载地址与 SHA256 摘要。
// File 通过 HTTP Range 把文件按块并发写入 .part 临时文件（服务器不支持
// Range 时退化为单线程流式），失败块自动重试，完成后校验 SHA256 再原子改名；
// 已存在的同名同大小且摘要一致的文件直接复用，避免重复下载。
package download

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LatestURL bangumi/Archive 最新导出快照元信息（JSON）。
const LatestURL = "https://raw.githubusercontent.com/bangumi/Archive/refs/heads/master/aux/latest.json"

const (
	userAgent     = "bangumi-subject-go/local"
	maxRounds     = 3               // 失败分块的最大重试轮数
	progressEvery = 2 * time.Second // 进度日志间隔
)

// minChunkSize 单块最小体积：避免小文件也开满连接数（测试中可调小）。
var minChunkSize = int64(8 << 20)

// Release aux/latest.json 中本程序关心的字段。
type Release struct {
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
	CreatedAt          string `json:"created_at"`
	Digest             string `json:"digest"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	UpdatedAt          string `json:"updated_at"`
}

func newClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   32,
		},
	}
}

// FetchLatest 拉取最新导出快照的元信息；url 通常传 LatestURL。
func FetchLatest(ctx context.Context, url string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := newClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 latest.json: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("latest.json 返回 HTTP %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("解析 latest.json: %w", err)
	}
	if rel.Name == "" || rel.BrowserDownloadURL == "" {
		return nil, errors.New("latest.json 缺少 name/browser_download_url 字段")
	}
	return &rel, nil
}

// chunkRange 一段待下载的字节区间（end 为闭区间），done 记录本区间已落盘字节。
type chunkRange struct {
	start, end int64
	done       atomic.Int64
	err        error
}

func (c *chunkRange) size() int64 { return c.end - c.start + 1 }

// File 多线程下载 rel 指向的压缩包到 dst 并校验 SHA256 摘要。
// dst 已存在且大小与摘要一致时跳过下载；数据先写入 dst+".part"，
// 校验通过后原子改名为 dst。threads 为并发连接数（<1 按 1 处理）。
func File(ctx context.Context, rel *Release, dst string, threads int) (err error) {
	if threads < 1 {
		threads = 1
	}

	// 已有完整文件：校验通过则复用
	if fi, statErr := os.Stat(dst); statErr == nil && fi.Size() == rel.Size && rel.Size > 0 {
		log.Printf("发现已有 %s，正在校验...", filepath.Base(dst))
		switch matchFile(dst, rel.Digest) {
		case true:
			log.Printf("校验一致，跳过下载")
			return nil
		case false:
			log.Printf("已有文件校验不一致，重新下载")
		}
	}

	size, ranged, err := probe(ctx, rel.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("探测下载地址: %w", err)
	}
	if size > 0 && rel.Size > 0 && size != rel.Size {
		return fmt.Errorf("远端大小 %d 与 latest.json 报告 %d 不一致", size, rel.Size)
	}

	partPath := dst + ".part"
	_ = os.Remove(partPath)
	f, err := os.Create(partPath)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			_ = os.Remove(partPath) // 失败不留半截文件，下次重来
		}
	}()

	started := time.Now()
	total := size
	var downloaded atomic.Int64
	stopLog := startProgressLogger(&downloaded, &total)
	defer close(stopLog)

	if ranged && size > 0 {
		log.Printf("开始多线程下载 %s（%s，%d 连接）", filepath.Base(dst), HumanBytes(size), threads)
		if err = f.Truncate(size); err != nil {
			return err
		}
		err = downloadRanges(ctx, f, rel.BrowserDownloadURL, size, threads, &downloaded)
	} else {
		log.Printf("服务器不支持断点分块，单线程流式下载 %s", filepath.Base(dst))
		err = singleStream(ctx, f, rel.BrowserDownloadURL, &downloaded)
	}
	if err != nil {
		return err
	}

	if err = verifyDigest(partPath, rel.Digest); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(partPath, dst); err != nil {
		return err
	}
	log.Printf("下载完成：%s（耗时 %s）", dst, time.Since(started).Round(time.Second))
	return nil
}

// startProgressLogger 周期性打印下载进度与速度；返回停止通道。
func startProgressLogger(downloaded *atomic.Int64, total *int64) chan struct{} {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(progressEvery)
		defer ticker.Stop()
		var last int64
		lastAt := time.Now()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				cur := downloaded.Load()
				speed := float64(cur-last) / time.Since(lastAt).Seconds()
				last, lastAt = cur, time.Now()
				if t := atomic.LoadInt64(total); t > 0 {
					log.Printf("下载进度 %.1f%%（%s/%s，%s/s）",
						float64(cur)/float64(t)*100, HumanBytes(cur), HumanBytes(t), HumanBytes(int64(speed)))
				} else {
					log.Printf("已下载 %s（%s/s）", HumanBytes(cur), HumanBytes(int64(speed)))
				}
			}
		}
	}()
	return stop
}

// splitChunks 将总大小均分为至多 threads 个不小于 minChunkSize 的闭区间块。
func splitChunks(size int64, threads int) []chunkRange {
	n := int64(threads)
	if max := (size + minChunkSize - 1) / minChunkSize; n > max {
		n = max
	}
	if n < 1 {
		n = 1
	}
	chunks := make([]chunkRange, n)
	base, extra := size/n, size%n
	var off int64
	for i := range chunks {
		l := base
		if int64(i) < extra {
			l++
		}
		chunks[i] = chunkRange{start: off, end: off + l - 1}
		off += l
	}
	return chunks
}

// downloadRanges 分块并发下载：每轮只取未完成的块，全部成功或重试轮数用尽为止。
func downloadRanges(ctx context.Context, f *os.File, url string, size int64, threads int, downloaded *atomic.Int64) error {
	chunks := splitChunks(size, threads)
	for round := 1; round <= maxRounds; round++ {
		var pending []int
		for i := range chunks {
			if chunks[i].err != nil || round == 1 {
				pending = append(pending, i)
			}
		}
		if len(pending) == 0 {
			break
		}
		if round > 1 {
			log.Printf("%d 个分块失败，第 %d 轮重试...", len(pending), round)
		}

		work := make(chan int, len(pending))
		for _, i := range pending {
			work <- i
		}
		close(work)

		workers := threads
		if workers > len(pending) {
			workers = len(pending)
		}
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range work {
					if ctx.Err() != nil {
						return
					}
					c := &chunks[i]
					if e := fetchChunk(ctx, f, url, c, downloaded); e != nil {
						// 重试前扣除该块已写入的字节，保证进度统计准确
						downloaded.Add(-c.done.Swap(0))
						c.err = e
						if ctx.Err() == nil {
							log.Printf("分块 [%d-%d] 下载失败: %v", c.start, c.end, e)
						}
						continue
					}
					c.err = nil
				}
			}()
		}
		wg.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	for i := range chunks {
		if chunks[i].err != nil {
			return fmt.Errorf("分块 [%d-%d] 多次重试后仍失败: %w", chunks[i].start, chunks[i].end, chunks[i].err)
		}
	}
	return nil
}

// fetchChunk 通过 Range 请求下载单个字节区间并写到文件的对应偏移处。
func fetchChunk(ctx context.Context, f *os.File, url string, c *chunkRange, downloaded *atomic.Int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", c.start, c.end))
	resp, err := newClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		return errors.New("服务器忽略了 Range 请求（返回完整内容）")
	default:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	length := c.size()
	if resp.ContentLength >= 0 && resp.ContentLength != length {
		return fmt.Errorf("响应长度 %d 与请求区间 %d 不符", resp.ContentLength, length)
	}
	w := &offsetWriter{f: f, off: c.start, done: &c.done, total: downloaded}
	n, err := io.CopyBuffer(w, resp.Body, make([]byte, 512<<10))
	if err != nil {
		return err
	}
	if n != length {
		return fmt.Errorf("读取 %d/%d 字节，连接提前中断", n, length)
	}
	return nil
}

// singleStream 不支持 Range 时的整文件流式下载。
func singleStream(ctx context.Context, f *os.File, url string, downloaded *atomic.Int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := newClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	w := &offsetWriter{f: f, done: &atomic.Int64{}, total: downloaded}
	_, err = io.CopyBuffer(w, resp.Body, make([]byte, 512<<10))
	return err
}

// offsetWriter 在文件的指定偏移处顺序写入，同时累计进度计数。
type offsetWriter struct {
	f     *os.File
	off   int64
	done  *atomic.Int64 // 本流已写字节
	total *atomic.Int64 // 全局进度
}

func (w *offsetWriter) Write(p []byte) (int, error) {
	n, err := w.f.WriteAt(p, w.off)
	w.off += int64(n)
	w.done.Add(int64(n))
	w.total.Add(int64(n))
	return n, err
}

// probe 探测下载地址：返回文件总大小与是否支持 Range 分块（206）。
func probe(ctx context.Context, url string) (size int64, ranged bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Range", "bytes=0-0")
	resp, err := newClient().Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
		cr := resp.Header.Get("Content-Range") // 形如 "bytes 0-0/434740633"
		i := strings.LastIndexByte(cr, '/')
		if i < 0 {
			return 0, false, fmt.Errorf("Content-Range 格式异常: %q", cr)
		}
		total, perr := strconv.ParseInt(cr[i+1:], 10, 64)
		if perr != nil || total <= 0 {
			return 0, false, fmt.Errorf("Content-Range 总大小异常: %q", cr)
		}
		return total, true, nil
	case http.StatusOK:
		return resp.ContentLength, false, nil
	default:
		return 0, false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
}

// parseDigest 解析 "sha256:<hex>" 形式的摘要；无法识别时返回 ok=false。
func parseDigest(d string) ([]byte, bool) {
	algo, hexVal, found := strings.Cut(d, ":")
	if !found || !strings.EqualFold(algo, "sha256") {
		return nil, false
	}
	raw, err := hex.DecodeString(hexVal)
	if err != nil || len(raw) != sha256.Size {
		return nil, false
	}
	return raw, true
}

// fileHash 计算文件内容的 SHA256。
func fileHash(path string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	f, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, bufio.NewReaderSize(f, 1<<20)); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// verifyDigest 校验已下载文件的 SHA256 与上游摘要一致；无摘要时提示后放行。
func verifyDigest(path, digest string) error {
	want, ok := parseDigest(digest)
	if !ok {
		log.Printf("上游未提供可识别的 SHA256 摘要，跳过完整性校验")
		return nil
	}
	sum, err := fileHash(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(sum[:], want) {
		return fmt.Errorf("SHA256 校验失败：期望 %x，实际 %x", want, sum)
	}
	return nil
}

// matchFile 判断已有文件是否与期望摘要一致；无摘要时视为一致（大小已在调用方比对）。
func matchFile(path, digest string) bool {
	want, ok := parseDigest(digest)
	if !ok {
		log.Printf("无摘要可比对，按大小视为有效")
		return true
	}
	sum, err := fileHash(path)
	if err != nil {
		log.Printf("计算已有文件摘要失败: %v", err)
		return false
	}
	return bytes.Equal(sum[:], want)
}

// HumanBytes 字节数的人读表示（如 414.6MB）。
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
