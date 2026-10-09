package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	diagnostichttp "github.com/AskarKasimov/ai-tutor/services/backend/internal/features/diagnostic/transport/http"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awsS3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type taskAudioTestAsset struct {
	id  string
	key string
}

func importCurrentTaskAudios(t *testing.T, f *fixture, filename string, data []byte, admin ...*http.Cookie) []taskAudioTestAsset {
	t.Helper()
	var cookie *http.Cookie
	if len(admin) == 0 {
		cookie = f.admin(t)
	} else {
		cookie = admin[0]
	}
	if w := upload(f, "/admin/competency-map/import", "file", filename, "text/csv", data, cookie); w.Code != http.StatusOK {
		t.Fatalf("import map for task audio: %d %s", w.Code, w.Body.String())
	}
	rows, err := f.pool.Query(context.Background(), `
SELECT asset.id, asset.object_key
FROM audio_assets asset
JOIN tasks task ON task.audio_asset_id = asset.id
JOIN outcomes outcome ON outcome.id = task.outcome_id
JOIN constituents constituent ON constituent.id = outcome.constituent_id
JOIN competencies competency ON competency.id = constituent.competency_id
JOIN competency_map_state state ON state.singleton=true AND state.revision=competency.revision
ORDER BY task.created_at, task.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []taskAudioTestAsset
	for rows.Next() {
		var asset taskAudioTestAsset
		if err := rows.Scan(&asset.id, &asset.key); err != nil {
			t.Fatal(err)
		}
		result = append(result, asset)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(result) == 0 {
		t.Fatal("map import created no current task audio assets")
	}
	return result
}

func keepOnlyQueuedAudio(t *testing.T, f *fixture, assets []taskAudioTestAsset, keep string) {
	t.Helper()
	for _, asset := range assets {
		if asset.id == keep {
			continue
		}
		if _, err := f.pool.Exec(context.Background(), "UPDATE audio_assets SET status='failed' WHERE id=$1 AND status='pending'", asset.id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTaskAudioIsSynthesizedOnceAndServedFromStorage(t *testing.T) {
	f := newFixture(t)
	var synthCalls atomic.Int32
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		synthCalls.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wavBytes())
	}))
	defer tts.Close()
	cfg := f.app.cfg
	cfg.TTSURL = tts.URL
	a, err := New(cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	a.now = f.app.now
	f.app = a
	f.handler = a.Handler()

	access, _, _ := f.register(t, "task-audio-once@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	variantRequest := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:intro-to-ml"}`))
	variantRequest.Header.Set("Content-Type", "application/json")
	variantRequest.Header.Set("Idempotency-Key", "task-audio-variant-1")
	variantRequest.AddCookie(access)
	variantResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(variantResponse, variantRequest)
	if variantResponse.Code != http.StatusCreated {
		t.Fatalf("create variant: %d %s", variantResponse.Code, variantResponse.Body.String())
	}
	var variant struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(variantResponse.Body.Bytes(), &variant); err != nil || variant.ID == "" {
		t.Fatalf("invalid variant: %s (%v)", variantResponse.Body.String(), err)
	}
	startBody, _ := json.Marshal(diagnostichttp.StartRequest{VariantID: variant.ID})
	startRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions", bytes.NewReader(startBody))
	startRequest.Header.Set("Content-Type", "application/json")
	startRequest.Header.Set("Idempotency-Key", "task-audio-session-1")
	startRequest.AddCookie(access)
	startResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusCreated {
		t.Fatalf("start session: %d %s", startResponse.Code, startResponse.Body.String())
	}
	var progress struct {
		SessionID string `json:"session_id"`
		Current   struct {
			ID string `json:"variant_task_id"`
		} `json:"current"`
	}
	if err := json.Unmarshal(startResponse.Body.Bytes(), &progress); err != nil || progress.Current.ID == "" {
		t.Fatalf("invalid session: %s (%v)", startResponse.Body.String(), err)
	}
	var audioID string
	if err := f.pool.QueryRow(context.Background(), "SELECT audio_asset_id FROM variant_tasks WHERE id=$1", progress.Current.ID).Scan(&audioID); err != nil || audioID == "" {
		t.Fatalf("audio asset: %q (%v)", audioID, err)
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE audio_assets SET status='failed' WHERE id <> $1 AND status='pending'", audioID); err != nil {
		t.Fatal(err)
	}

	readMetadata := func() *httptest.ResponseRecorder {
		return f.request(http.MethodGet, "/diagnostic-sessions/"+progress.SessionID+"/current/audio?variant_task_id="+progress.Current.ID, "", access)
	}
	pending := readMetadata()
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), `"status":"pending"`) || synthCalls.Load() != 0 {
		t.Fatalf("metadata triggered synthesis or wrong status: %d %s; TTS calls=%d", pending.Code, pending.Body.String(), synthCalls.Load())
	}
	processed, err := f.app.audioWorker.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("worker ProcessOne: processed=%v err=%v", processed, err)
	}
	if got := synthCalls.Load(); got != 1 {
		t.Fatalf("TTS calls after worker: %d, want 1", got)
	}
	var status, bucket, storageURI, audioURL string
	if err := f.pool.QueryRow(context.Background(), "SELECT status,bucket,storage_uri,audio_url FROM audio_assets WHERE id=$1", audioID).Scan(&status, &bucket, &storageURI, &audioURL); err != nil {
		t.Fatal(err)
	}
	if status != "ready" || bucket != cfg.S3Bucket || !strings.HasPrefix(storageURI, "s3://"+cfg.S3Bucket+"/") || audioURL != "/task-audio/"+audioID+"/file" {
		t.Fatalf("persisted audio metadata: status=%q bucket=%q uri=%q url=%q", status, bucket, storageURI, audioURL)
	}
	ready := readMetadata()
	if ready.Code != http.StatusOK || !strings.Contains(ready.Body.String(), `"status":"ready"`) || !strings.Contains(ready.Body.String(), `"audio_url":"`+audioURL+`"`) {
		t.Fatalf("ready metadata: %d %s", ready.Code, ready.Body.String())
	}
	for i := 0; i < 2; i++ {
		file := f.request(http.MethodGet, audioURL, "", access)
		if file.Code != http.StatusOK || !bytes.Equal(file.Body.Bytes(), wavBytes()) {
			t.Fatalf("saved file read %d: %d %q", i, file.Code, file.Body.String())
		}
	}
	if got := synthCalls.Load(); got != 1 {
		t.Fatalf("HTTP reads synthesized again: TTS calls=%d", got)
	}
	if w := upload(f, "/admin/competency-map/import", "file", "replacement.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("reimport: %d %s", w.Code, w.Body.String())
	}
	var liveTaskID *string
	if err := f.pool.QueryRow(context.Background(), "SELECT live_task_id FROM variant_tasks WHERE id=$1", progress.Current.ID).Scan(&liveTaskID); err != nil || liveTaskID != nil {
		t.Fatalf("reimport did not retire historical task: live_task_id=%v err=%v", liveTaskID, err)
	}
	var retainedStatus string
	if err := f.pool.QueryRow(context.Background(), "SELECT status FROM audio_assets WHERE id=$1", audioID).Scan(&retainedStatus); err != nil || retainedStatus != "ready" {
		t.Fatalf("reimport lost saved audio: status=%q err=%v", retainedStatus, err)
	}
	oldVariantBody, _ := json.Marshal(diagnostichttp.StartRequest{VariantID: variant.ID})
	oldVariantRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions", bytes.NewReader(oldVariantBody))
	oldVariantRequest.Header.Set("Content-Type", "application/json")
	oldVariantRequest.Header.Set("Idempotency-Key", "task-audio-old-variant-after-reimport")
	oldVariantRequest.AddCookie(access)
	oldVariantResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(oldVariantResponse, oldVariantRequest)
	var oldProgress struct {
		SessionID string `json:"session_id"`
		Current   struct {
			ID string `json:"variant_task_id"`
		} `json:"current"`
	}
	if err := json.Unmarshal(oldVariantResponse.Body.Bytes(), &oldProgress); oldVariantResponse.Code != http.StatusCreated || err != nil || oldProgress.Current.ID != progress.Current.ID {
		t.Fatalf("old variant after reimport: %d %s err=%v", oldVariantResponse.Code, oldVariantResponse.Body.String(), err)
	}
	oldAudio := f.request(http.MethodGet, "/diagnostic-sessions/"+oldProgress.SessionID+"/current/audio?variant_task_id="+oldProgress.Current.ID, "", access)
	if oldAudio.Code != http.StatusOK || !strings.Contains(oldAudio.Body.String(), `"status":"ready"`) || !strings.Contains(oldAudio.Body.String(), `"audio_url":"`+audioURL+`"`) {
		t.Fatalf("old variant did not retain ready task audio: %d %s", oldAudio.Code, oldAudio.Body.String())
	}
	other, _, _ := f.register(t, "task-audio-foreign@example.edu")
	if foreign := f.request(http.MethodGet, audioURL, "", other); foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign account read historical audio: %d %s", foreign.Code, foreign.Body.String())
	}
}

func TestRegenerateCurrentAudioRepairsMissingObjectSynchronously(t *testing.T) {
	f := newFixture(t)
	var synthCalls atomic.Int32
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		synthCalls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request["text"] != "Назовите ответ" {
			t.Errorf("repair used unexpected TTS text: %v err=%v", request, err)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wavBytes())
	}))
	defer tts.Close()
	cfg := f.app.cfg
	cfg.TTSURL = tts.URL
	a, err := New(cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	a.now = f.app.now
	f.app, f.handler = a, a.Handler()

	access, _, _ := f.register(t, "audio-repair@example.edu")
	assets := importCurrentTaskAudios(t, f, "repair-map.csv", variantMapCSV(t))
	if len(assets) == 0 {
		t.Fatal("import created no tasks")
	}
	variantRequest := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:intro-to-ml"}`))
	variantRequest.Header.Set("Content-Type", "application/json")
	variantRequest.Header.Set("Idempotency-Key", "audio-repair-variant")
	variantRequest.AddCookie(access)
	variantResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(variantResponse, variantRequest)
	if variantResponse.Code != http.StatusCreated {
		t.Fatalf("create variant: %d %s", variantResponse.Code, variantResponse.Body.String())
	}
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(variantResponse.Body.Bytes(), &v); err != nil || v.ID == "" {
		t.Fatalf("variant response: %s %v", variantResponse.Body.String(), err)
	}
	startBody, _ := json.Marshal(diagnostichttp.StartRequest{VariantID: v.ID})
	startRequest := httptest.NewRequest(http.MethodPost, "https://api.example/diagnostic-sessions", bytes.NewReader(startBody))
	startRequest.Header.Set("Content-Type", "application/json")
	startRequest.Header.Set("Idempotency-Key", "audio-repair-session")
	startRequest.AddCookie(access)
	start := httptest.NewRecorder()
	f.handler.ServeHTTP(start, startRequest)
	if start.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", start.Code, start.Body.String())
	}
	var progress struct {
		SessionID string `json:"session_id"`
		Current   struct {
			ID string `json:"variant_task_id"`
		} `json:"current"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &progress); err != nil {
		t.Fatal(err)
	}
	var audioID string
	if err := f.pool.QueryRow(context.Background(), `SELECT audio_asset_id FROM variant_tasks WHERE id=$1`, progress.Current.ID).Scan(&audioID); err != nil || audioID == "" {
		t.Fatalf("current task audio id=%q err=%v", audioID, err)
	}
	var asset taskAudioTestAsset
	for _, candidate := range assets {
		if candidate.id == audioID {
			asset = candidate
			break
		}
	}
	if asset.id == "" {
		t.Fatalf("current audio %q was not imported", audioID)
	}
	keepOnlyQueuedAudio(t, f, assets, asset.id)
	if _, err := f.pool.Exec(context.Background(), `UPDATE audio_assets SET status='ready',bucket=$2,storage_uri=$3,audio_url=$4 WHERE id=$1`, asset.id, cfg.S3Bucket, "s3://"+cfg.S3Bucket+"/"+asset.key, "/task-audio/"+asset.id+"/file"); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(diagnostichttp.AudioRegenerationRequest{VariantTaskID: progress.Current.ID})
	response := f.request(http.MethodPost, "/diagnostic-sessions/"+progress.SessionID+"/current/audio/regenerate", string(body), access)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ready"`) || synthCalls.Load() != 1 {
		t.Fatalf("repair response=%d %s TTS calls=%d", response.Code, response.Body.String(), synthCalls.Load())
	}
	file := f.request(http.MethodGet, "/task-audio/"+asset.id+"/file", "", access)
	if file.Code != http.StatusOK || !strings.HasPrefix(file.Body.String(), "RIFF") || synthCalls.Load() != 1 {
		t.Fatalf("file after repair=%d bytes=%d TTS calls=%d", file.Code, file.Body.Len(), synthCalls.Load())
	}
}

func TestTaskAudioWorkerRecoversUploadedObjectAndRetriesHeadFailure(t *testing.T) {
	f := newFixture(t)
	assets := importCurrentTaskAudios(t, f, "recovery-map.csv", variantMapCSV(t))
	if len(assets) < 2 {
		t.Fatal("recovery tests require two current tasks")
	}
	recoveredID, recoveredKey := assets[0].id, assets[0].key
	failingID := assets[1].id
	keepOnlyQueuedAudio(t, f, assets, recoveredID)
	var synthCalls atomic.Int32
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		synthCalls.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wavBytes())
	}))
	defer tts.Close()
	cfg := f.app.cfg
	cfg.TTSURL = tts.URL
	a, err := New(cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	f.app = a
	if err := f.app.audioStorage.Put(context.Background(), cfg.S3Bucket, recoveredKey, recoveredID, wavBytes()); err != nil {
		t.Fatalf("simulate object uploaded before crash: %v", err)
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE audio_assets SET status='processing', claim_token='expired-worker', lease_until=now()-interval '1 second', attempts=1 WHERE id=$1", recoveredID); err != nil {
		t.Fatal(err)
	}
	processed, err := f.app.audioWorker.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("recovery ProcessOne: processed=%v err=%v", processed, err)
	}
	var status string
	if err := f.pool.QueryRow(context.Background(), "SELECT status FROM audio_assets WHERE id=$1", recoveredID).Scan(&status); err != nil || status != "ready" {
		t.Fatalf("recovered asset status=%q err=%v", status, err)
	}
	if synthCalls.Load() != 0 {
		t.Fatalf("crash recovery called TTS %d times", synthCalls.Load())
	}

	if _, err := f.pool.Exec(context.Background(), "UPDATE audio_assets SET status='pending',attempts=0 WHERE id=$1", failingID); err != nil {
		t.Fatal(err)
	}
	f.s3Mu.Lock()
	f.s3HeadStatus = http.StatusInternalServerError
	f.s3Mu.Unlock()
	processed, err = f.app.audioWorker.ProcessOne(context.Background())
	if !processed || err == nil {
		t.Fatalf("HEAD failure: processed=%v err=%v", processed, err)
	}
	var failureStatus, errorCode string
	if err := f.pool.QueryRow(context.Background(), "SELECT status,last_error_code FROM audio_assets WHERE id=$1", failingID).Scan(&failureStatus, &errorCode); err != nil || failureStatus != "pending" || errorCode != "storage_head_failed" {
		t.Fatalf("HEAD failure transition: status=%q error=%q err=%v", failureStatus, errorCode, err)
	}
	if synthCalls.Load() != 0 {
		t.Fatalf("HEAD failure called TTS %d times", synthCalls.Load())
	}
}

func TestTaskAudioReplacementCancelsSnapshotOnlyAndExpiredAssets(t *testing.T) {
	f := newFixture(t)
	access, _, _ := f.register(t, "task-audio-replacement@example.edu")
	admin := f.admin(t)
	if w := upload(f, "/admin/competency-map/import", "file", "map-a.csv", "text/csv", variantMapCSV(t), admin); w.Code != http.StatusOK {
		t.Fatalf("import A: %d %s", w.Code, w.Body.String())
	}
	variantRequest := httptest.NewRequest(http.MethodPost, "https://api.example/variants", strings.NewReader(`{"subject_id":"subject:intro-to-ml"}`))
	variantRequest.Header.Set("Content-Type", "application/json")
	variantRequest.Header.Set("Idempotency-Key", "task-audio-replacement-variant")
	variantRequest.AddCookie(access)
	variantResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(variantResponse, variantRequest)
	var variant struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(variantResponse.Body.Bytes(), &variant); variantResponse.Code != http.StatusCreated || err != nil || variant.ID == "" {
		t.Fatalf("create A snapshot: %d %s err=%v", variantResponse.Code, variantResponse.Body.String(), err)
	}
	rows, err := f.pool.Query(context.Background(), "SELECT DISTINCT audio_asset_id FROM variant_tasks WHERE variant_id=$1 AND audio_asset_id IS NOT NULL", variant.ID)
	if err != nil {
		t.Fatal(err)
	}
	var oldAssetIDs []string
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		oldAssetIDs = append(oldAssetIDs, assetID)
	}
	rows.Close()
	if len(oldAssetIDs) == 0 {
		t.Fatal("variant A has no snapshot audio assets")
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE audio_assets SET status='processing',claim_token='expired-a',lease_until=now()-interval '1 second',attempts=1 WHERE id=$1", oldAssetIDs[0]); err != nil {
		t.Fatal(err)
	}
	mapB := bytes.ReplaceAll(variantMapCSV(t), []byte("Назовите ответ."), []byte("Новая инструкция B."))
	if w := upload(f, "/admin/competency-map/import", "file", "map-b.csv", "text/csv", mapB, admin); w.Code != http.StatusOK {
		t.Fatalf("replace with B: %d %s", w.Code, w.Body.String())
	}
	for _, id := range oldAssetIDs[1:] {
		var status string
		if err := f.pool.QueryRow(context.Background(), "SELECT status FROM audio_assets WHERE id=$1", id).Scan(&status); err != nil || status != "cancelled" {
			t.Fatalf("snapshot-only pending A %q status=%q err=%v", id, status, err)
		}
	}
	var synthText string
	ttsCalls := atomic.Int32{}
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode TTS request: %v", err)
		}
		synthText = body.Text
		ttsCalls.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wavBytes())
	}))
	defer tts.Close()
	cfg := f.app.cfg
	cfg.TTSURL = tts.URL
	a, err := New(cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	f.app = a
	processed, err := f.app.audioWorker.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("worker after replacement: processed=%v err=%v", processed, err)
	}
	var expiredStatus string
	if err := f.pool.QueryRow(context.Background(), "SELECT status FROM audio_assets WHERE id=$1", oldAssetIDs[0]).Scan(&expiredStatus); err != nil || expiredStatus != "cancelled" {
		t.Fatalf("expired snapshot-only A status=%q err=%v", expiredStatus, err)
	}
	if ttsCalls.Load() != 1 || !strings.Contains(synthText, "Новая инструкция B") {
		t.Fatalf("replacement synthesized wrong task: calls=%d text=%q", ttsCalls.Load(), synthText)
	}
}

func TestTaskAudioWorkerUsesMockSynthesizerWithPostgresAndS3(t *testing.T) {
	f := newFixture(t)
	assets := importCurrentTaskAudios(t, f, "mock-map.csv", variantMapCSV(t))
	assetID, objectKey := assets[0].id, assets[0].key
	keepOnlyQueuedAudio(t, f, assets, assetID)
	cfg := f.app.cfg
	cfg.APIMode = "mock"
	a, err := New(cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	f.app = a
	processed, err := f.app.audioWorker.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("mock ProcessOne: processed=%v err=%v", processed, err)
	}
	var status, bucket string
	if err := f.pool.QueryRow(context.Background(), "SELECT status,bucket FROM audio_assets WHERE id=$1", assetID).Scan(&status, &bucket); err != nil || status != "ready" || bucket != cfg.S3Bucket {
		t.Fatalf("mock asset status=%q bucket=%q err=%v", status, bucket, err)
	}
	file, info, err := f.app.audioStorage.Open(context.Background(), bucket, objectKey)
	if err != nil {
		t.Fatalf("mock S3 object not readable: %v", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read mock object: %v", err)
	}
	if len(data) == 0 || info.AssetID != assetID || info.ContentType != "audio/wav" || !bytes.HasPrefix(data, []byte("RIFF")) {
		t.Fatalf("mock object metadata/data: bytes=%d info=%+v", len(data), info)
	}
}

func TestTaskAudioPostgresRustFSSmoke(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT is not set")
	}
	accessKey, secretKey := os.Getenv("TEST_S3_ACCESS_KEY"), os.Getenv("TEST_S3_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		t.Fatal("TEST_S3_ACCESS_KEY and TEST_S3_SECRET_KEY are required with TEST_S3_ENDPOINT")
	}
	f := newFixture(t)
	assets := importCurrentTaskAudios(t, f, "rustfs-map.csv", variantMapCSV(t))
	assetID, objectKey := assets[0].id, assets[0].key
	keepOnlyQueuedAudio(t, f, assets, assetID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	bucket := "codex-e2e-smoke-" + suffix
	cfg := f.app.cfg
	cfg.APIMode = "mock"
	cfg.S3Endpoint, cfg.S3Bucket = endpoint, bucket
	cfg.S3AccessKey, cfg.S3SecretKey = accessKey, secretKey
	cfg.S3CreateBucket = true
	a, err := New(cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	f.app = a
	cleanupClient := awsS3.NewFromConfig(aws.Config{Region: cfg.S3Region, Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")}, func(options *awsS3.Options) {
		options.UsePathStyle = true
		options.BaseEndpoint = aws.String(endpoint)
	})
	defer func() {
		_, _ = cleanupClient.DeleteObject(context.Background(), &awsS3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)})
		_, _ = cleanupClient.DeleteBucket(context.Background(), &awsS3.DeleteBucketInput{Bucket: aws.String(bucket)})
	}()
	if err := a.audioStorage.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure RustFS smoke bucket: %v", err)
	}
	processed, err := a.audioWorker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("PostgreSQL/RustFS worker: processed=%v err=%v", processed, err)
	}
	var status, savedBucket, storageURI string
	if err := f.pool.QueryRow(ctx, "SELECT status,bucket,storage_uri FROM audio_assets WHERE id=$1", assetID).Scan(&status, &savedBucket, &storageURI); err != nil || status != "ready" || savedBucket != bucket || !strings.HasPrefix(storageURI, "s3://"+bucket+"/") {
		t.Fatalf("persisted RustFS metadata: status=%q bucket=%q uri=%q err=%v", status, savedBucket, storageURI, err)
	}
	body, info, err := a.audioStorage.Open(ctx, bucket, objectKey)
	if err != nil {
		t.Fatalf("read RustFS worker object: %v", err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || len(data) == 0 || !bytes.HasPrefix(data, []byte("RIFF")) || info.AssetID != assetID {
		t.Fatalf("RustFS worker bytes/metadata: size=%d info=%+v err=%v", len(data), info, err)
	}
}

func TestTaskAudioLargeQueueBoundsClaimsAndDoesNotBlockImport(t *testing.T) {
	f := newFixture(t)
	admin := f.admin(t)
	baseAssets := importCurrentTaskAudios(t, f, "large-map-a.csv", variantMapCSV(t), admin)
	var outcomeID string
	if err := f.pool.QueryRow(context.Background(), `SELECT task.outcome_id FROM tasks task WHERE task.audio_asset_id=$1 LIMIT 1`, baseAssets[0].id).Scan(&outcomeID); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 100)
	release := make(chan struct{}, 100)
	var active, maximum atomic.Int32
	tts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wavBytes())
	}))
	defer tts.Close()
	cfg := f.app.cfg
	cfg.TTSURL = tts.URL
	cfg.AudioWorkers = 2
	a, err := New(cfg, f.pool, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	a.now = f.app.now
	f.app = a
	for i := 0; i < 100; i++ {
		assetID := fmt.Sprintf("large-queue-%03d", i)
		taskID := fmt.Sprintf("large-task-%03d", i)
		if _, err := f.pool.Exec(context.Background(), "INSERT INTO audio_assets(id,instruction,object_key) VALUES ($1,$2,$3)", assetID, "Скажи ответ", "task-audio/v1/"+assetID+".wav"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(context.Background(), "INSERT INTO tasks(id,outcome_id,question,voice_instruction,created_at,audio_asset_id) VALUES ($1,$2,'Question','Скажи ответ',1,$3)", taskID, outcomeID, assetID); err != nil {
			t.Fatal(err)
		}
	}
	workerResults := make(chan error, 2)
	for range 2 {
		go func() {
			processed, err := f.app.audioWorker.ProcessOne(context.Background())
			if err == nil && !processed {
				err = fmt.Errorf("worker found no queued audio")
			}
			workerResults <- err
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("two worker lanes did not reach blocked TTS")
		}
	}
	var processing int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM audio_assets WHERE status='processing'").Scan(&processing); err != nil || processing != 2 {
		close(release)
		t.Fatalf("claimed processing assets=%d err=%v; want exactly 2", processing, err)
	}
	importDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		importDone <- upload(f, "/admin/competency-map/import", "file", "new-map.csv", "text/csv", variantMapCSV(t), admin)
	}()
	select {
	case response := <-importDone:
		if response.Code != http.StatusOK {
			close(release)
			t.Fatalf("import while TTS blocked: %d %s", response.Code, response.Body.String())
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("import waited for the worker network request to finish")
	}
	for range 2 {
		release <- struct{}{}
	}
	for range 2 {
		if err := <-workerResults; err != nil {
			t.Fatalf("ProcessOne: %v", err)
		}
	}
	if got := maximum.Load(); got != 2 {
		t.Fatalf("maximum concurrent TTS calls=%d, want 2", got)
	}
}
