package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	_ "embed"
)

//go:embed baseprompt.json
var basePromptRaw []byte

func fetchUserInfoWithToken(token string) (map[string]interface{}, error) {
	req, _ := http.NewRequest("GET", "https://openapi.qoder.sh/api/v1/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type bridge struct {
	sess         *sessionContext
	client       *bearerClient
	templateBase map[string]interface{}
	apiMode      string // "openai" | "anthropic" | "gemini" | "claude-code"
}

// newBridge 创建 API 转换桥接
// 支持两种认证方式：
// 1. OAuth device token (dt-xxx): 直接使用，调用 /api/v1/userinfo 获取用户信息
// 2. Personal Access Token (PAT): 调用 exchangeJobToken 转换为 session token
func newBridge(pat string, apiMode string) (*bridge, error) {
	mid := newUUID()
	mtoken := newBase64Token()
	mtype := newHexToken(18)

	log.Printf("[Bridge] Using token: %s (prefix: %s)", pat[:10]+"...", pat[:4])

	var identity authIdentity
	var name, id string

	// 判断 token 类型：dt- 开头是 device token，直接使用
	if strings.HasPrefix(pat, "dt-") {
		// OAuth device token：直接使用，不调用 exchangeJobToken
		log.Printf("[Bridge] Using OAuth device token directly")
		// 使用 device token 获取用户信息
		userInfo, err := fetchUserInfoWithToken(pat)
		if err != nil {
			return nil, fmt.Errorf("fetch user info: %w", err)
		}
		name = strVal(userInfo, "name")
		id = strVal(userInfo, "userId")
		identity = authIdentity{
			Name:               name,
			Aid:                id,
			Uid:                id,
			UserType:           strValDefault(userInfo, "userType", "personal_standard"),
			SecurityOauthToken: pat, // device token
			RefreshToken:       "",
		}
	} else {
		// PAT：调用 exchangeJobToken
		jt, err := exchangeJobToken(pat, mid, mtoken, mtype)
		if err != nil {
			return nil, fmt.Errorf("exchangeJobToken: %w", err)
		}
		name = strVal(jt, "name")
		id = strVal(jt, "id")
		identity = authIdentity{
			Name:               name,
			Aid:                id,
			Uid:                id,
			UserType:           strValDefault(jt, "userType", "personal_standard"),
			SecurityOauthToken: strVal(jt, "securityOauthToken"),
			RefreshToken:       strVal(jt, "refreshToken"),
		}
	}

	fmt.Printf("[bridge] session for %s (%s)\n", name, id)
	sess, err := newSession(identity, mid, mtoken, mtype)
	if err != nil {
		return nil, err
	}
	client := newBearerClient(sess)

	// prepare template: replace placeholders
	tmpl := string(basePromptRaw)
	for _, ukey := range []string{"{UUID1}", "{UUID2}", "{UUID3}", "{UUID4}", "{UUID5}"} {
		tmpl = strings.ReplaceAll(tmpl, ukey, newUUID())
	}
	tmpl = strings.ReplaceAll(tmpl, "{TIME1}", fmt.Sprintf("%d", unixMs()))

	var templateBase map[string]interface{}
	if err := json.Unmarshal([]byte(tmpl), &templateBase); err != nil {
		return nil, fmt.Errorf("parse baseprompt: %w", err)
	}

	return &bridge{
		sess:         sess,
		client:       client,
		templateBase: templateBase,
		apiMode:      apiMode,
	}, nil
}

func (b *bridge) start(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", b.handleChat)
	mux.HandleFunc("/v1/messages", b.handleMessages)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("[bridge] listening http://%s/v1/chat/completions\n", addr)
	fmt.Printf("[bridge] listening http://%s/v1/messages  (Anthropic)\n", addr)
	return http.ListenAndServe(addr, mux)
}

// deepCopyMap does a JSON round-trip deep copy
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(m)
	var out map[string]interface{}
	json.Unmarshal(data, &out)
	return out
}

func (b *bridge) callQoder(ctx context.Context, prompt, model string, onChunk func(string)) error {
	body := deepCopyMap(b.templateBase)

	nid := newUUID()
	body["request_id"] = nid
	body["chat_record_id"] = nid
	body["request_set_id"] = newUUID()
	body["session_id"] = newUUID()
	body["stream"] = true
	body["aliyun_user_type"] = b.sess.Identity.UserType

	if mc, ok := body["model_config"].(map[string]interface{}); ok {
		mc["key"] = model
	}
	if biz, ok := body["business"].(map[string]interface{}); ok {
		biz["id"] = newUUID()
		biz["begin_at"] = unixMs()
		if len(prompt) > 30 {
			biz["name"] = prompt[:30]
		} else {
			biz["name"] = prompt
		}
	}

	if cc, ok := body["chat_context"].(map[string]interface{}); ok {
		if txt, ok := cc["text"].(map[string]interface{}); ok {
			txt["text"] = prompt
		}
		if extra, ok := cc["extra"].(map[string]interface{}); ok {
			if oc, ok := extra["originalContent"].(map[string]interface{}); ok {
				oc["text"] = prompt
			}
		}
	}

	// rebuild messages: keep non-user, add new user message
	var rebuilt []interface{}
	if msgs, ok := body["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]interface{}); ok {
				if mm["role"] != "user" {
					rebuilt = append(rebuilt, m)
				}
			}
		}
	}
	userMsg := map[string]interface{}{
		"role":    "user",
		"content": "",
		"contents": []interface{}{
			map[string]interface{}{"type": "text", "text": prompt},
		},
		"response_meta": map[string]interface{}{
			"id": "",
			"usage": map[string]interface{}{
				"prompt_tokens":             0,
				"completion_tokens":         0,
				"total_tokens":              0,
				"completion_tokens_details": map[string]interface{}{"reasoning_tokens": 0},
				"prompt_tokens_details":     map[string]interface{}{"cached_tokens": 0},
			},
		},
		"reasoning_content_signature": "",
	}
	rebuilt = append(rebuilt, userMsg)
	body["messages"] = rebuilt

	mcSource := "system"
	if mc, ok := body["model_config"].(map[string]interface{}); ok {
		if s, ok := mc["source"].(string); ok {
			mcSource = s
		}
	}

	qurl := "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation" +
		"?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	extra := map[string]string{
		"x-model-key":    model,
		"x-model-source": mcSource,
	}

	preview := prompt
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	fmt.Printf("[bridge] prompt=%s\n", preview)

	return b.client.openStreamLines(ctx, qurl, body, extra, func(line string) {
		// 跳过非 data 行（如 event:、注释、空行等）
		if !strings.HasPrefix(line, "data:") {
			return
		}
		dataPayload := strings.TrimSpace(line[5:])
		// [DONE] 标记由调用方处理
		if dataPayload == "[DONE]" {
			return
		}
		chunk := extractContent(dataPayload)
		if chunk != "" {
			onChunk(chunk)
		}
	})
}

// extractContent 从 Qoder SSE data 行中提取文本内容。
// 支持多种响应格式：
//   - 标准格式: {"body": "{\"choices\":...}"}
//   - 扁平格式: {"choices": [...]}
//   - 直接 body 字符串
//
// 非文本事件（如心跳、错误、状态变更）静默跳过。
func extractContent(dataLine string) string {
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &wrapper); err != nil {
		return ""
	}

	// 尝试提取 body 字段
	inner, _ := wrapper["body"].(string)
	var innerJSON map[string]interface{}
	if inner != "" {
		if err := json.Unmarshal([]byte(inner), &innerJSON); err != nil {
			// body 不是合法 JSON，尝试直接作为文本内容
			return inner
		}
	} else {
		// 没有 body 字段，wrapper 本身就是内层数据
		innerJSON = wrapper
	}

	// 尝试从 choices 中提取内容
	choices, _ := innerJSON["choices"].([]interface{})
	for _, ch := range choices {
		chm, _ := ch.(map[string]interface{})
		// 优先取 delta.content（流式）
		if delta, ok := chm["delta"].(map[string]interface{}); ok {
			if c, ok := delta["content"].(string); ok && c != "" {
				return c
			}
			// reasoning_content（推理模型）
			if rc, ok := delta["reasoning_content"].(string); ok && rc != "" {
				return rc
			}
		}
		// 再取 message.content（非流式）
		if msg, ok := chm["message"].(map[string]interface{}); ok {
			if c, ok := msg["content"].(string); ok && c != "" {
				return c
			}
		}
	}

	// 没有 choices，可能是其他类型事件（心跳、状态等），静默跳过
	return ""
}

func (b *bridge) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, err)
		return
	}
	stream, _ := req["stream"].(bool)
	model := strValDefault(req, "model", "lite")
	prompt := extractLastUserOpenAI(req)

	reqId := "chatcmpl-" + newRequestId()
	created := unixSec()

	// 使用请求 context，当客户端断开时自动取消上游流式读取
	ctx := r.Context()

	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		err = b.callQoder(ctx, prompt, model, func(chunk string) {
			ck := makeChunk(reqId, created, model)
			choices := ck["choices"].([]interface{})
			delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
			delta["role"] = "assistant"
			delta["content"] = chunk
			data, _ := json.Marshal(ck)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			if flusher != nil {
				flusher.Flush()
			}
		})
		if err != nil {
			log.Printf("callQoder error: %v", err)
			// 发送错误事件通知客户端，而非静默截断
			errData, _ := json.Marshal(map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("stream interrupted: %v", err),
					"type":    "qoder_stream_error",
				},
			})
			fmt.Fprintf(w, "data: %s\n\n", string(errData))
			if flusher != nil {
				flusher.Flush()
			}
		}
		// done chunk
		done := makeChunk(reqId, created, model)
		choices := done["choices"].([]interface{})
		ch := choices[0].(map[string]interface{})
		ch["finish_reason"] = "stop"
		ch["delta"] = map[string]interface{}{}
		data, _ := json.Marshal(done)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", string(data))
		if flusher != nil {
			flusher.Flush()
		}
	} else {
		var full strings.Builder
		err = b.callQoder(ctx, prompt, model, func(chunk string) { full.WriteString(chunk) })
		if err != nil {
			writeErr(w, fmt.Errorf("request failed: %w", err))
			return
		}
		resp := map[string]interface{}{
			"id": reqId, "object": "chat.completion",
			"created": created, "model": model,
			"choices": []interface{}{
				map[string]interface{}{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": full.String()},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		}
		writeJSON(w, resp)
	}
}

func (b *bridge) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, err)
		return
	}
	stream, _ := req["stream"].(bool)
	model := strValDefault(req, "model", "lite")
	prompt := extractLastUserAnthropic(req)

	msgId := "msg_" + newRequestId()

	// 使用请求 context，当客户端断开时自动取消上游流式读取
	ctx := r.Context()

	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		writeSse := func(eventType string, data interface{}) {
			d, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(d))
			if flusher != nil {
				flusher.Flush()
			}
		}

		writeSse("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id": msgId, "type": "message", "role": "assistant", "model": model,
				"stop_reason": nil, "stop_sequence": nil,
				"content": []interface{}{},
				"usage":   map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
			},
		})
		writeSse("content_block_start", map[string]interface{}{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]interface{}{"type": "text", "text": ""},
		})
		writeSse("ping", map[string]interface{}{"type": "ping"})

		err = b.callQoder(ctx, prompt, model, func(chunk string) {
			writeSse("content_block_delta", map[string]interface{}{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]interface{}{"type": "text_delta", "text": chunk},
			})
		})
		if err != nil {
			log.Printf("callQoder error: %v", err)
		}

		writeSse("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": 0})
		writeSse("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": "end_turn", "stop_sequence": nil},
			"usage": map[string]interface{}{"output_tokens": 0},
		})
		writeSse("message_stop", map[string]interface{}{"type": "message_stop"})
	} else {
		var full strings.Builder
		err = b.callQoder(ctx, prompt, model, func(chunk string) { full.WriteString(chunk) })
		if err != nil {
			writeErr(w, fmt.Errorf("request failed: %w", err))
			return
		}
		resp := map[string]interface{}{
			"id": msgId, "type": "message", "role": "assistant", "model": model,
			"stop_reason": "end_turn", "stop_sequence": nil,
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": full.String()},
			},
			"usage": map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
		}
		writeJSON(w, resp)
	}
}

func makeChunk(id string, created int64, model string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": nil,
			},
		},
	}
}

func extractLastUserOpenAI(req map[string]interface{}) string {
	msgs, _ := req["messages"].([]interface{})
	for i := len(msgs) - 1; i >= 0; i-- {
		m, _ := msgs[i].(map[string]interface{})
		if m["role"] == "user" {
			c := m["content"]
			if s, ok := c.(string); ok {
				return s
			}
			data, _ := json.Marshal(c)
			return string(data)
		}
	}
	return ""
}

func extractLastUserAnthropic(req map[string]interface{}) string {
	msgs, _ := req["messages"].([]interface{})
	for i := len(msgs) - 1; i >= 0; i-- {
		m, _ := msgs[i].(map[string]interface{})
		if m["role"] == "user" {
			return extractAnthropicText(m["content"])
		}
	}
	return ""
}

func extractAnthropicText(content interface{}) string {
	if s, ok := content.(string); ok {
		return s
	}
	arr, ok := content.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, block := range arr {
		b, _ := block.(map[string]interface{})
		if b["type"] == "text" {
			sb.WriteString(strVal(b, "text"))
		}
	}
	return sb.String()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	data, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

func writeErr(w http.ResponseWriter, err error) {
	msg := err.Error()
	msg = strings.ReplaceAll(msg, `"`, `\"`)
	body := fmt.Sprintf(`{"error":{"message":"%s","type":"qoder_error"}}`, msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(500)
	w.Write([]byte(body))
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func strValDefault(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

// handleGemini handles Gemini API format (placeholder)
func (b *bridge) handleGemini(w http.ResponseWriter, r *http.Request) {
	// Gemini API 格式转换（待实现）
	// 目前返回未实现错误
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(501)
	w.Write([]byte(`{"error":{"message":"Gemini API mode not implemented yet","type":"not_implemented"}}`))
}
