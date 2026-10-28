package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultSubUA = "v2rayN/7.10.0"

type SubInfo struct {
	Title          string `json:"title,omitempty"`
	Upload         int64  `json:"upload,omitempty"`
	Download       int64  `json:"download,omitempty"`
	Total          int64  `json:"total,omitempty"`
	Expire         int64  `json:"expire,omitempty"`
	UpdateInterval int    `json:"updateInterval,omitempty"` // часы, из profile-update-interval
	SupportURL     string `json:"supportUrl,omitempty"`
	Via            string `json:"via"` // direct | proxy
}

type SubResult struct {
	Servers []*Server `json:"servers"`
	Info    SubInfo   `json:"info"`
	Skipped int       `json:"skipped"`
}

// FetchSubscription скачивает подписку. Сначала напрямую, а если не вышло и ядро
// запущено — через локальный прокси (на случай, если домен подписки заблокирован).
func FetchSubscription(ctx context.Context, rawURL, ua string, proxyPort int) (*SubResult, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("ссылка подписки должна начинаться с http:// или https://")
	}
	if ua == "" {
		ua = defaultSubUA
	}
	body, hdr, errDirect := httpGet(ctx, u.String(), ua, nil)
	via := "direct"
	if errDirect != nil && proxyPort > 0 {
		proxyURL, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(proxyPort))
		var errProxy error
		body, hdr, errProxy = httpGet(ctx, u.String(), ua, proxyURL)
		if errProxy != nil {
			return nil, fmt.Errorf("напрямую: %v; через прокси: %v", errDirect, errProxy)
		}
		via = "proxy"
	} else if errDirect != nil {
		return nil, errDirect
	}
	res, err := ParseSubscription(body)
	if err != nil {
		return nil, err
	}
	res.Info = parseSubHeaders(hdr)
	res.Info.Via = via
	return res, nil
}

func httpGet(ctx context.Context, u, ua string, proxy *url.URL) ([]byte, http.Header, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != nil {
		tr.Proxy = http.ProxyURL(proxy)
	}
	cl := &http.Client{Transport: tr, Timeout: 25 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	resp, err := cl.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("сервер ответил %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, err
	}
	return b, resp.Header, nil
}

// ParseSubscription понимает: base64-список ссылок, обычный список ссылок, JSON-конфиг sing-box.
func ParseSubscription(body []byte) (*SubResult, error) {
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")))
	if len(body) == 0 {
		return nil, errors.New("подписка пустая")
	}
	res := &SubResult{}
	if body[0] == '{' {
		servers, err := serversFromSingBoxJSON(body)
		if err != nil {
			return nil, fmt.Errorf("не удалось разобрать JSON подписки: %v", err)
		}
		res.Servers = dedupe(servers)
		if len(res.Servers) == 0 {
			return nil, errors.New("в JSON подписки нет поддерживаемых серверов")
		}
		return res, nil
	}
	text := string(body)
	if !strings.Contains(text, "://") {
		dec, err := decodeB64(text)
		if err != nil {
			if strings.HasPrefix(text, "proxies:") || strings.Contains(text, "\nproxies:") {
				return nil, errors.New("подписка в формате Clash YAML — укажите в настройках другой User-Agent")
			}
			return nil, errors.New("не удалось распознать формат подписки")
		}
		text = string(dec)
	}
	var list []*Server
	for _, line := range strings.Split(text, "\n") {
		s, err := ParseLink(line)
		if err == errSkip {
			continue
		}
		if err != nil {
			res.Skipped++
			continue
		}
		list = append(list, s)
	}
	res.Servers = dedupe(list)
	if len(res.Servers) == 0 {
		return nil, errors.New("в подписке не найдено ни одного сервера")
	}
	return res, nil
}

func parseSubHeaders(h http.Header) SubInfo {
	var info SubInfo
	if h == nil {
		return info
	}
	for _, part := range strings.Split(h.Get("Subscription-Userinfo"), ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		n, _ := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64)
		switch strings.ToLower(kv[0]) {
		case "upload":
			info.Upload = int64(n)
		case "download":
			info.Download = int64(n)
		case "total":
			info.Total = int64(n)
		case "expire":
			info.Expire = int64(n)
		}
	}
	if t := h.Get("Profile-Title"); t != "" {
		if strings.HasPrefix(t, "base64:") {
			if b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(t, "base64:")); err == nil {
				t = string(b)
			}
		}
		info.Title = t
	} else if cd := h.Get("Content-Disposition"); cd != "" {
		if i := strings.Index(cd, "filename="); i >= 0 {
			info.Title = strings.Trim(cd[i+len("filename="):], `"' `)
		}
	}
	if v, err := strconv.Atoi(strings.TrimSpace(h.Get("Profile-Update-Interval"))); err == nil {
		info.UpdateInterval = v
	}
	info.SupportURL = h.Get("Support-Url")
	return info
}
