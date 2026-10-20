package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type qrResponse struct {
	Token string `json:"token"`
}

type result struct {
	latencies []float64
	success   int64
	failed    int64
	statuses  sync.Map
}

func main() {
	base := flag.String("base", "http://localhost:8080", "базовый URL backend")
	loginValue := flag.String("login", "", "логин администратора")
	password := flag.String("password", "", "пароль администратора")
	employees := flag.Int("employees", 150, "количество сотрудников (операторов)")
	rate := flag.Int("rate", 60, "суммарная интенсивность отметок в секунду")
	duration := flag.Duration("duration", 30*time.Minute, "длительность теста")
	prefix := flag.String("prefix", "LOAD", "префикс табельных номеров")
	flag.Parse()

	if *loginValue == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "нужны -login и -password администратора")
		os.Exit(2)
	}

	transport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 1000,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport}
	token, err := doLogin(client, *base, *loginValue, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "вход: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("вход выполнен, создаю %d сотрудников...\n", *employees)
	tokens, err := prepareEmployees(client, *base, token, *prefix, *employees)
	if err != nil {
		fmt.Fprintf(os.Stderr, "подготовка: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("готово: %d QR-токенов. Старт: %d отм/с, %s\n", len(tokens), *rate, *duration)

	var res result
	var latMu sync.Mutex
	stop := make(chan struct{})
	var wg sync.WaitGroup
	perWorker := float64(*rate) / float64(len(tokens))
	if perWorker < 0.05 {
		perWorker = 0.05
	}
	interval := time.Duration(float64(time.Second) / perWorker)

	for _, qr := range tokens {
		wg.Add(1)
		go func(qrToken string) {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					start := time.Now()
					status, err := createMark(client, *base, token, qrToken)
					elapsed := float64(time.Since(start).Microseconds()) / 1000
					latMu.Lock()
					res.latencies = append(res.latencies, elapsed)
					latMu.Unlock()
					if err == nil && status >= 200 && status < 300 {
						atomic.AddInt64(&res.success, 1)
					} else {
						atomic.AddInt64(&res.failed, 1)
					}
					res.statuses.Store(status, true)
				}
			}
		}(qr)
	}

	time.Sleep(*duration)
	close(stop)
	wg.Wait()

	if len(res.latencies) == 0 {
		fmt.Println("не выполнено ни одного запроса")
		return
	}
	sort.Float64s(res.latencies)
	pick := func(q float64) float64 {
		index := int(q * float64(len(res.latencies)-1))
		return res.latencies[index]
	}
	total := res.success + res.failed
	seconds := duration.Seconds()
	fmt.Println("=== Итог нагрузочного теста ===")
	fmt.Printf("запросов: %d (успешно %d, ошибок %d)\n", total, res.success, res.failed)
	fmt.Printf("фактическая интенсивность: %.1f отм/с\n", float64(total)/seconds)
	fmt.Printf("задержка p50: %.1f мс\n", pick(0.50))
	fmt.Printf("задержка p95: %.1f мс (цель <= 300 мс)\n", pick(0.95))
	fmt.Printf("задержка p99: %.1f мс\n", pick(0.99))
	fmt.Printf("максимум: %.1f мс\n", res.latencies[len(res.latencies)-1])
	var codes []int
	res.statuses.Range(func(key, _ any) bool {
		if code, ok := key.(int); ok && code != 0 {
			codes = append(codes, code)
		}
		return true
	})
	sort.Ints(codes)
	fmt.Printf("коды ответов: %v\n", codes)
}

func randomID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(buf[0:4]), hex.EncodeToString(buf[4:6]),
		hex.EncodeToString(buf[6:8]), hex.EncodeToString(buf[8:10]),
		hex.EncodeToString(buf[10:16]))
}

func doLogin(client *http.Client, base, loginValue, password string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"login": loginValue, "password": password})
	resp, err := client.Post(base+"/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var pair struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &pair); err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

func prepareEmployees(client *http.Client, base, token, prefix string, count int) ([]string, error) {
	tokens := make([]string, 0, count)
	for i := 0; i < count; i++ {
		number := fmt.Sprintf("%s-%06d-%s", prefix, i, randomID()[:8])
		body, _ := json.Marshal(map[string]any{"employee_number": number, "full_name": "Нагрузочный " + number})
		req, _ := http.NewRequest(http.MethodPost, base+"/admin/employees", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			return nil, fmt.Errorf("создание сотрудника HTTP %d: %s", resp.StatusCode, raw)
		}
		var employee struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &employee); err != nil {
			return nil, err
		}
		qrBody, _ := json.Marshal(map[string]any{"employee_id": employee.ID, "payload": map[string]any{}})
		qrReq, _ := http.NewRequest(http.MethodPost, base+"/admin/qr/static", bytes.NewReader(qrBody))
		qrReq.Header.Set("Authorization", "Bearer "+token)
		qrReq.Header.Set("Content-Type", "application/json")
		qrResp, err := client.Do(qrReq)
		if err != nil {
			return nil, err
		}
		qrRaw, _ := io.ReadAll(qrResp.Body)
		qrResp.Body.Close()
		if qrResp.StatusCode != http.StatusCreated {
			return nil, fmt.Errorf("выпуск QR HTTP %d: %s", qrResp.StatusCode, qrRaw)
		}
		var qr qrResponse
		if err := json.Unmarshal(qrRaw, &qr); err != nil {
			return nil, err
		}
		tokens = append(tokens, qr.Token)
	}
	return tokens, nil
}

func createMark(client *http.Client, base, token, qrToken string) (int, error) {
	body, _ := json.Marshal(map[string]any{"qr_token": qrToken, "client_type": "web"})
	req, _ := http.NewRequest(http.MethodPost, base+"/mark", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", randomID())
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, nil
}
