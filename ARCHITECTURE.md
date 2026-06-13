# Luna Bot — Architecture Document

## 1. Overview

Luna Bot is a Telegram chatbot with autonomous behaviour (Free Will), emotional analysis, personality memory, cognitive architecture, and social relationship tracking. The bot uses local LLM (Gemma 4 12B via Ollama) as the primary provider, with Gemini/DeepSeek/OpenRouter as fallbacks.

---

## 2. System Architecture (High-Level)

```
┌─────────────────────────────────────────────────────┐
│                   Telegram API                       │
└─────────────┬───────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────┐
│  Bot (main loop)                                    │
│  ├── OnMessage → FreeWill.OnMessage                 │
│  ├── OnDirectMention → FreeWill.OnDirectMention     │
│  ├── Responder (legacy) ↙ (degraded path)          │
│  └── EmotionalAnalyzer (background ticker)          │
└─────────────┬───────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────┐
│  FreeWill Service (core autonomous module)          │
│  ├── analyzeAndAct (2-stage decision)               │
│  ├── analyzeDirectResponse (direct mentions)        │
│  ├── CheckSilence (ticker-based silence detection)  │
│  ├── updateMood (LLM-based mood analysis)           │
│  └── reactions (emoji reactions)                    │
└─────────────┬───────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────┐
│  State Machines                                     │
│  ├── PresenceState (online/asleep/busy)             │
│  ├── ConflictState (argument detection)             │
│  ├── RelationshipStage + Score                      │
│  └── MoodState (circadian rhythm, sin wave)         │
└─────────────┬───────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────┐
│  LLM Layer                                          │
│  ├── Local provider (Ollama, Gemma 4 12B Unc.)     │
│  ├── Gemini / DeepSeek / OpenRouter fallbacks       │
│  ├── ResponseType-based routing                     │
│  └── Circuit Breaker                                │
└─────────────┬───────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────┐
│  Storage (interface)                                │
│  ├── PostgreSQL (main, many stubs)                  │
│  ├── FileStorage (legacy)                           │
│  └── MockStorage (tests)                            │
└─────────────────────────────────────────────────────┘
```

---

## 3. LLM Response Types (decisionResponseTypes)

Defined in `internal/local/provider.go:61-78`. Types marked as `decisionResponseTypes` get **low temperature (0.2)** and disabled TopP — intended for structured JSON decisions:

| Type | Purpose |
|------|---------|
| `free_will_should_reply` | Stage 1: should bot reply? |
| `free_will_response_type` | Stage 2: what type of reply? |
| `free_will_direct_response_decision` | Direct mention: should reply? |
| `free_will_mood_analysis` | LLM-based mood update |
| `free_will_reaction` | Emoji reaction decision |
| `reaction_analysis` | Reaction analysis |
| `web_search_trigger` | Web search trigger |
| `classify` | Message classification |
| `moderation` | Content moderation |
| `srach` | Conflict detection |
| `causal_analysis` | Causal learning |
| `causal_influence` | Causal influence |
| **`emotional_analysis`** | Emotional analysis |
| **`emotional_adaptation`** | Emotional adaptation |
| **`emotional_feedback`** | Emotional feedback |
| `belief_analysis` | Belief analysis |

---

## 4. Free Will Module

### 4.1 Two-Stage Architecture

```
Message received
  │
  ├── Quick Rules (deterministic, no LLM)
  │   ├── asleep → skip
  │   ├── conflict-cold → skip
  │   ├── busy → skip
  │   └── night-awake → reply (general, tired)
  │
  ├── Stage 1: decideShouldReply (LLM, temp=0.2)
  │   └── Returns JSON: {should_reply, reply_type, target_message_id, reason}
  │
  ├── Stage 2: decideResponseType (LLM, temp=0.2)
  │   └── Returns JSON: {text, is_voice, mood}
  │
  └── executeDecision
      ├── voice message (TTS)
      └── text message
```

### 4.2 Mood in Free Will
- `updateMood()` — LLM-based (free_will_mood_analysis), probabilistic (10% chance per check)
- `getCurrentMood()` — reads from `storage.MoodState` via `GetMoodState()` (STUB in Postgres)
- Used in all prompt builders: `{mood}` and `{intensity}` placeholders

---

## 5. State Machines

### 5.1 PresenceState
- `state/presence.go`
- Computes: online, asleep, night_awake, busy status
- Based on: circadian profile, local timezone, busy slots

### 5.2 ConflictState
- `state/conflict.go`
- Tracks conflict level (0-5), cold_active status
- Influences: mood (irritability), quick rules (skip replies)

### 5.3 RelationshipStage
- `state/relationship.go`
- Stages: StageMetIrlGotTg → StageTgGivenWarming → StageConvinced → StageLongTerm
- Score: interest, trust, attraction, annoyance, cringe

### 5.4 MoodState (Circadian)
- `state/mood.go`
- **Purely mathematical** — sin wave based on local hour
- Parameters: energy, irritability, affection, current_mood
- Used in: system prompt fragment (`MoodPromptFragment`)
- **NOT connected to EmotionalAnalyzer**

---

## 6. Personality Memory
- `PersonalityMemory` stored per chat
- Contains: aliases, topics, self-perceptions, emotional memories
- Updated periodically via personality analysis LLM calls
- Integrated into prompts via `enrichPromptWithPersonality()`

---

## 7. Cognitive Architecture
- Internal Monologue (Stage 3): self-reflection before responding
- Self-Reflection: periodic introspection
- Confidence Calibration: track prediction accuracy

---

## 8. Social Architecture
- Relationship tracking: trust/intimacy per user
- Social learning: adjust behaviour based on feedback
- Relationship-influenced communication style

---

## 9. Web Search
- Google Custom Search API integration
- Cached results (TTL: 5 min)
- Used for serious direct responses

---

## 10. Anti-Repetition System
- Embedding-based similarity detection
- Cosine distance threshold: 0.75
- Local rework (no LLM) for short texts
- LLM-based rework for longer texts

---

## 11. TTS (Text-to-Speech)
- Primary: ElevenLabs
- Fallback: Gemini TTS
- Voice: Arcadias
- Used for Free Will voice replies

---

## 12. Storage Layer

### 12.1 Interface (`storage/storage.go`)
Core storage interface with ~60+ methods.

### 12.2 Implementations
| Implementation | Status | Use |
|---------------|--------|-----|
| PostgresStorage | Partial (many STUBS) | Main |
| FileStorage | Full | Legacy |
| MockStorage | Full | Tests |

### 12.3 Stubbed Methods (PostgreSQL)
All emotional methods are STUBS (return nil/no-op with WARN log):
- `GetEmotionalState` / `SaveEmotionalState` / `UpdateEmotionalState`
- `AddEmotionalMemory` / `GetEmotionalMemories` / `GetEmotionalMemoriesByEmotion`
- `UpdateEmotionalMemory` / `CleanupEmotionalMemories` / `GetEmotionalTrends`

All mood methods are STUBS or return defaults:
- `GetMoodState` (returns default neutral)
- `SaveMoodState` (no-op with DEBUG log)
- `UpdateMoodState` (no-op with DEBUG log)

---

## 13. Phase 11: Emotional System (Full Audit)

### 13.1 Architectural Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    EMOTIONAL SYSTEM (DUAL)                       │
│                                                                  │
│  ┌──────────────────────┐     ┌──────────────────────────────┐  │
│  │  MoodState (carcadian) │     │ EmotionalAnalyzer (LLM-based) │  │
│  │  state/mood.go         │     │ emotional_analyzer.go         │  │
│  │                        │     │                               │  │
│  │ • SIN wave energy      │     │ • Ticker: every 2h           │  │
│  │ • Irritability base    │     │ • Per-chat, per-user         │  │
│  │ • Affection calc       │     │ • minMessages: 3 (too low!)  │  │
│  │ • Conflict influence   │     │ • LLM call: emotional_analysis│  │
│  │ • Relationship stage   │     │ • LLM call: emotional_adapt. │  │
│  │                        │     │ • LLM call: emotional_feedback│  │
│  │  NO LLM — pure math    │     │                               │  │
│  │  NOT persisted via     │     │  ALL results → STUBS!         │  │
│  │  MoodState table       │     │  (9 methods in Postgres)      │  │
│  └──────────┬─────────────┘     └──────────────┬───────────────┘  │
│             │                                   │                  │
│             │    THEY DO NOT CONNECT!            │                  │
│             │    ╔═══════════════╗              │                  │
│             └────╣ NO INTERFACE ╠──────────────┘                  │
│                  ╚═══════════════╝                                 │
│                                                                  │
│  System Prompt uses MoodState (math)                              │
│  EmotionalAnalyzer results go to STUBS (lost)                     │
│  EmotionalResponseAdapter is DEAD CODE                            │
└─────────────────────────────────────────────────────────────────┘
```

### 13.2 LLM Calls Map

| Call | Trigger | Interval | Context | ResponseType | Temp | Status |
|------|---------|----------|---------|--------------|------|--------|
| Emotional Analysis | Ticker | 2h | XML messages (all + user) | `emotional_analysis` | 0.2 (decision!) | Runs, result lost to stub |
| Emotional Adaptation | Per-response | Every reply | EmotionalContext (stubs → empty) | `emotional_adaptation` | 0.2 (decision!) | Runs, but context empty |
| Emotional Feedback | Per-user reaction | After response | Interaction + reaction | `emotional_feedback` | 0.2 (decision!) | Runs, result lost to stub |
| Mood Analysis (FW) | Probabilistic | ~10% per message | ChatML context | `free_will_mood_analysis` | 0.2 | Runs, saves to stub |

### 13.3 Storage Schema (proposed, NOT implemented)

**Table: `emotional_states`**
```sql
chat_id, joy, sadness, anger, fear, trust, disgust, surprise,
anticipation, optimism, contempt, nostalgia, anxiety, aggression,
sentimentality, curiosity, cynicism, uncertainty, empathy,
irritability, vulnerability, confidence, response_tendency (jsonb),
intensity, stability, last_update, trigger_event
```

**Table: `emotional_memories`**
```sql
chat_id, user_id, user_context, trigger, primary_emotion,
emotion_intensity, response, outcome, success, reinforcement,
frequency, last_accessed, topic_context, keywords
```

**Table: `mood_states`**
```sql
chat_id, current_mood, mood_intensity, last_mood_update, trigger_reason
```

### 13.4 Problems and Root Causes

| # | Problem | Cause | Severity |
|---|---------|-------|----------|
| 1 | **All emotional data lost** | 9 Postgres stub methods | CRITICAL |
| 2 | **Two mood systems, zero integration** | MoodState (math) vs EmotionalAnalyzer (LLM) never merged | CRITICAL |
| 3 | **EmotionalResponseAdapter DEAD CODE** | `getEmotionallyInfluencedResponseType()` never called | HIGH |
| 4 | **Emotional analysis blocks Free Will** | Ticker runs `analyzeEmotionsForAllChats()` in same goroutine pool, competing for LLM | HIGH |
| 5 | **Analysis threshold too low** | `minMessages: 3` triggers LLM for tiny contexts | HIGH |
| 6 | **Emotional adaptation context always empty** | Depends on `GetEmotionalContext` → `GetEmotionalMemories` → STUB returns nil | MEDIUM |
| 7 | **Emotional types in decisionResponseTypes** | Uses temp=0.2 (decision) instead of 0.6-0.8 (appropriate for nuanced emotional analysis) | MEDIUM |  
| 8 | **Per-user analysis, not per-chat** | N users in chat = N LLM calls instead of 1 | MEDIUM |
| 9 | **No debounce per user** | Same user analyzed every 2h regardless of activity | LOW |
| 10 | **XML formatting overhead** | Full XML formatting even for emotional context (heavy) | LOW |

### 13.5 Why "Two Mood Systems" Is a Problem

```
User sends message → Bot needs to answer
                        │
        ┌───────────────┴────────────────┐
        ▼                                 ▼
  System Prompt:                    Free Will mood:
  MoodState (math)                  storage.MoodState (LLM)
  "energy: high"                    "current_mood: sarcastic"
  "irritability: low"               "intensity: 0.7"
        │                                 │
        └───────────────┬────────────────┘
                        ▼
               TWO DIFFERENT MOODS
               IN ONE RESPONSE
                        │
               The bot's "personality brain"
               is SPLIT IN TWO
```

The system prompt gets **mathematical** mood (circadian sin wave), while Free Will prompt builders get **LLM-analyzed** mood (from `getCurrentMood`). These two sources can and do contradict — e.g. the math says "energetic" (it's 12:00), but LLM says "irritated" (someone was arguing). The bot gets confused signals.

### 13.6 Proposed Optimizations

#### P11.1: Remove emotional from decisionResponseTypes
- Move `emotional_analysis`, `emotional_adaptation`, `emotional_feedback` out of `decisionResponseTypes`
- They need creative temperature (0.6-0.8), not decision temperature (0.2)

#### P11.2: Full audit (THIS TASK)
- Document complete system
- Identify all problems
- Plan integration

#### P11.3: Implement storage
- Create `emotional_states` table in PostgreSQL
- Create `emotional_memories` table
- Implement all 9 stub methods
- Implement `mood_states` table methods

#### P11.4: Raise analysis thresholds
- `minMessages`: 3 → 20
- Add per-user debounce: 6h minimum between analyses
- Add per-chat debounce: don't re-analyze if context hasn't changed

#### P11.5: Hybrid Mood (critical integration)
- Combine Plutchik scores (from LLM) with circadian sin wave (math)
- Formula: `final_mood = emotional_scores × 0.4 + circadian_mood × 0.6`
- Single `MoodState` struct feeds BOTH system prompt and Free Will
- Kills the "split brain" problem

#### P11.6: Async analysis
- Emotional ticker runs in dedicated goroutine
- Uses separate LLM call budget (not competing with response generation)
- Non-blocking: results applied asynchronously

#### P11.7: Per-chat analysis
- Instead of N per-user LLM calls, do 1 per-chat analysis
- LLM receives all messages, returns per-user scores in one JSON

#### P11.8: Caching
- Cache emotional analysis results per user
- Don't re-analyze if no new messages since last analysis
- TTL: 6h per user

### 13.7 Integration Plan

```
Phase 1 (P11.3): Storage
  └── Implement Postgres tables + methods

Phase 2 (P11.4): Thresholds
  └── Raise minMessages, add debounce

Phase 3 (P11.1): Temperature fix
  └── Remove emotional types from decisionResponseTypes

Phase 4 (P11.6-8): Performance
  └── Async, per-chat, caching

Phase 5 (P11.5): Hybrid Mood (unification)
  └── Merge LLM emotional scores with math mood
  └── Single MoodState struct
  └── Remove DEAD CODE (EmotionalResponseAdapter)
```

---

## Appendix: Configuration Overview

| Section | Key Settings |
|---------|-------------|
| `llm` | Providers, fallback, circuit breaker, response_types routing |
| `telegram` | Token, admin IDs, timezone, bot_names |
| `chat` | Context window (300), daily take time |
| `summary` | Interval (4h), weekly settings |
| `free_will` | Min/max intervals, mood probability, voice probability, silence detection |
| `emotional_learning` | Enabled, analysis interval (2h), lookback (100), retention (30d) |
| `personality` | Update interval (1h), lookback (50) |
| `cognitive_architecture` | Internal monologue, self-reflection, confidence calibration |
| `social_architecture` | Relationship tracking, social learning, intimacy/trust rates |
| `storage` | Type (postgres), connection settings |

---

## 15. Phase 13: Унификация LLM-провайдеров

### 15.1 Текущее состояние (аудит)

**6 провайдеров, 2 активны.** Остальные 4 (deepseek, openrouter, openai, anthropic) отключены в YAML (`enabled: false`), но их код загружается, занимает память и усложняет поддержку.

**Сводка per-provider:**

| Провайдер | Файл | Строк | Timeout | Retry | KeyRot | Capabilities |
|-----------|------|-------|---------|-------|--------|-------------|
| local | `internal/local/client.go` | 312 | 300s HTTP, 300s/30s ctx | max=1, timeout only, 500ms | нет | TextGen |
| gemini | `internal/gemini/client.go` | 1605 | **30s HTTP** (КРИТИЧНО!) | нет | есть (429→reserve) | ALL 6 |
| deepseek | `internal/deepseek/client.go` | 305 | lib default | нет | нет | TextGen |
| openrouter | `internal/openrouter/client.go` | 424 | 120s HTTP, 150s ctx | нет | нет | TextGen |
| openai | `internal/openai/client.go` | 284 | 120s HTTP, 150s ctx | нет | нет | TextGen |
| anthropic | `internal/anthropic/client.go` | 276 | 120s HTTP, 150s ctx | нет | нет | TextGen |

**Ключевые проблемы:**

1. **Gemini timeout = 30s** — катастрофически мал. Для CPU-bound локальной модели 300s нормально, но Gemini 2.0 Flash отвечает за 5-15s. При 30s любой микрозамедление вызывает timeout.
2. **Retry только у local** — жёстко захардкожен в `client.go:276-291` (maxRetries=1, только timeout-ошибки, sleep 500ms). Остальные провайдеры падают с первой ошибки.
3. **Fallback-ссылки ведут в пустоту** — `fallback_provider_order: ["local", "gemini", "deepseek", "openrouter"]`, но deepseek и openrouter отключены. `free_will_should_reply.fallback_provider: "deepseek"` — dead reference.
4. **54+ response_types** — 80% записей идентичны (`provider: "local"`, `model: ""`, `fallback_provider: ""`), различается только temperature. `post_process_*` типы (5 записей) — DEPRECATED, удалены из кода, но висят в YAML.
5. **Per-provider таймауты захардкожены** в каждом конструкторе (`http.Client{Timeout: N * time.Second}`). Нельзя изменить без перекомпиляции.
6. **isRetryableError проверяет строки** (`strings.Contains(errStr, "429")`) — fragile, не ловит `context.DeadlineExceeded`, `net.Error`.

### 15.2 Целевая архитектура: ProviderManager

**Принцип:** Вся retry/timeout/CB-логика выносится из клиентов провайдеров в единый слой `ProviderManager` внутри `LLMRouterV2`. Клиенты становятся «чистыми» — только HTTP-вызов, без retry и собственных таймаутов (кроме транспортного HTTP-таймаута как safety net).

```
Запрос (LLMRouterV2.GenerateResponseByType)
  │
  ├── RoutingProfile (из YAML) определяет провайдера и модель
  │
  ├── ProviderManager.tryWithRetry(name, fn)
  │   │
  │   ├── Шаг 0: CircuitBreaker.Allow() → если OPEN → сразу ошибка
  │   ├── Шаг 1: context.WithTimeout(ctx, provider.Config.RequestTimeout)
  │   ├── Шаг 2: retry loop (max_retries из конфига)
  │   │   ├── попытка N: fn(ctx)  // вызов provider.Generate(...)
  │   │   ├── успех → CB.RecordSuccess() → return result
  │   │   ├── retryable? (timeout/429/5xx)
  │   │   │   ├── да → sleep(retry_delay * backoff^N) → continue
  │   │   │   └── нет → CB.RecordFailure() → return error
  │   │   └── попытки исчерпаны → CB.RecordFailure() → return error
  │   └── KeyRotation: при 429 → rotate → retry с новым ключом
  │
  ├── Primary провайдер → tryWithRetry
  ├── Fallback провайдер → tryWithRetry (если primary — retryable ошибка)
  └── Full chain (все TextGenerator'ы) → tryWithRetry для каждого
```

**ProviderManager — НЕ отдельный тип.** Это набор методов на `LLMRouterV2`:

```go
// RetryConfig — настройки повторов для провайдера.
type RetryConfig struct {
    MaxRetries        int     // 0 = без повторов
    RetryDelayMs      int     // начальная задержка
    BackoffMultiplier float64 // множитель экспоненциального backoff
}

// tryWithRetry выполняет вызов с таймаутом, retry и circuit breaker.
func (r *LLMRouterV2) tryWithRetry(
    providerName string,
    fn func(ctx context.Context) (string, error),
) (string, error)
```

### 15.3 Конфигурация per-provider (YAML)

Новые поля в `ProviderConfig`:

```yaml
providers:
  local:
    enabled: true
    request_timeout_seconds: 600   # CPU-bound, нужно много времени
    retry:
      max_retries: 2
      retry_delay_ms: 1000
      backoff_multiplier: 2.0
    # ... остальные поля без изменений

  gemini:
    enabled: true
    request_timeout_seconds: 120   # БЫЛО 30! 120s достаточно для 2.0 Flash
    retry:
      max_retries: 3
      retry_delay_ms: 2000
      backoff_multiplier: 2.0
    # ...

  deepseek:
    enabled: false                 # отключён — не инициализируется
    request_timeout_seconds: 120
    retry:
      max_retries: 2
      retry_delay_ms: 1000
      backoff_multiplier: 2.0
```

Соответствующие изменения в `config.ProviderConfig`:

```go
type ProviderConfig struct {
    // ...existing fields...
    RequestTimeoutSeconds int         `yaml:"request_timeout_seconds"`
    Retry                 RetryConfig `yaml:"retry"`
}

type RetryConfig struct {
    MaxRetries        int     `yaml:"max_retries"`
    RetryDelayMs      int     `yaml:"retry_delay_ms"`
    BackoffMultiplier float64 `yaml:"backoff_multiplier"`
}
```

**Значения по умолчанию** (если не заданы в YAML):
- `request_timeout_seconds`: 120
- `retry.max_retries`: 2
- `retry.retry_delay_ms`: 1000
- `retry.backoff_multiplier`: 2.0

### 15.4 Улучшенный isRetryableError

Текущая версия (`llm_router_v2.go:237-248`) проверяет только подстроки в `err.Error()`. Новая версия:

```go
func (r *LLMRouterV2) isRetryableError(err error) bool {
    if err == nil {
        return false
    }
    // context timeout
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }
    // net.Error (tcp timeout, dns timeout, etc.)
    var netErr net.Error
    if errors.As(err, &netErr) && netErr.Timeout() {
        return true
    }
    // HTTP status codes via error string (провайдеры оборачивают статус в ошибку)
    errStr := err.Error()
    for _, code := range []string{"429", "500", "502", "503", "504"} {
        if strings.Contains(errStr, code) {
            return true
        }
    }
    // circuit breaker open → не retryable на этом провайдере,
    // но retryable для fallback
    if strings.Contains(errStr, "circuit breaker") {
        return true
    }
    return false
}
```

### 15.5 Упрощение response_types

**Проблема:** 54+ записей в `response_types` с ~80% дублированием. Каждая запись — 6 строк YAML = ~320 строк мусора.

**Предлагаемая логика:** `GetRoutingProfile` использует каскадный поиск:

1. Точное совпадение в `response_types` → полный override
2. Нет точного совпадения → взять `default` и применить partial override из секции `response_type_overrides` (только изменённые поля)
3. Нет `default` → hardcoded fallback (`provider: default_provider`, `temperature: 1.0`)

**Результат:** Вместо 54+ записей — ~10-12, только те что реально отличаются:

```yaml
response_types:
  # Базовый профиль для всех типов без явного override
  default:
    provider: "local"
    temperature: 1.0

  # Decision types — Gemini (low latency kritisch) + low temp
  free_will_should_reply:
    provider: "gemini"
    temperature: 0.2
    fallback_provider: "local"       # БЫЛО "deepseek" (отключён!)

  free_will_response_type:
    provider: "gemini"
    temperature: 0.2
    fallback_provider: "local"

  free_will_direct_response_decision:
    provider: "gemini"
    temperature: 0.2
    fallback_provider: "local"

  # Low-temp аналитические
  classify:              {temperature: 0.3}
  moderation:            {temperature: 0.3}
  voice_format:          {temperature: 0.3}
  personality_name:      {temperature: 0.3}
  srach_confirm:         {temperature: 0.3}

  # Mid-temp
  personality_topic:     {temperature: 0.4}
  internal_monologue:    {temperature: 0.4}
  web_search:            {temperature: 0.4}
  free_will_reaction:    {temperature: 0.4}

  srach_warning:         {temperature: 0.5}
  srach_analysis:        {temperature: 0.5}
  reaction_analysis:     {temperature: 0.5}
  personality_analysis:  {temperature: 0.5}
  self_reflection:       {temperature: 0.5}
  belief_analysis:       {temperature: 0.5}
  free_will_mood:        {temperature: 0.5}

  auto_bio:              {temperature: 0.6}
  auto_bio_update:       {temperature: 0.6}
  personality_self:      {temperature: 0.6}
  relationship_analysis: {temperature: 0.6}

  summary:               {temperature: 0.7}
  photo_analysis:        {temperature: 0.7}
  causal_analysis:       {temperature: 0.7}
  image_gen:             {temperature: 0.7}

  serious:               {temperature: 0.8}
  emotional_analysis:    {temperature: 0.8}
  emotional_adaptation:  {temperature: 0.7}
  emotional_feedback:    {temperature: 0.6}

  anti_repetition:       {temperature: 1.1}

  daily_take:            {temperature: 1.2}
  free_will_silence:     {temperature: 1.2}
```

**Удалить из YAML (DEPRECATED, не используются в коде):**
- `post_process_single`, `post_process_short`, `post_process_long`, `post_process_intelligent`, `post_process_summary`

### 15.6 Fallback-логика (очистка)

**Текущее состояние:**
```yaml
fallback_provider_order:
  - "local"
  - "gemini"
  - "deepseek"     # отключён!
  - "openrouter"   # отключён!
```

**После очистки:**
```yaml
fallback_provider_order:
  - "local"
  - "gemini"
```

**Алгоритм fallback в LLMRouterV2** (без изменений, только чистка ссылок):

```
1. primary = RoutingProfile.Provider
2. fallback = RoutingProfile.FallbackProvider
3. full_chain = fallback_provider_order (все включённые TextGenerator'ы)

Для каждого запроса:
  tryWithRetry(primary)
  если retryable ошибка → tryWithRetry(fallback)
  если retryable ошибка → tryWithRetry(каждый из full_chain, кроме уже опробованных)
  если не-retryable → сразу ошибка (нет смысла пробовать другие)
```

**Critical path (free_will decision types):** Gemini primary → local fallback.
**Все остальные:** local primary → gemini fallback.

### 15.7 Отключение не-текстовых функций

**YAML-флаги:**
```yaml
voice_messages:
  enabled: false          # отключить голосовые сообщения

tts:
  provider: ""            # пустая строка = TTS отключён

free_will:
  image_generation:
    max_per_interval: 0   # 0 = отключить генерацию изображений
```

**Code-level блокировка (refreshCaches):**
```go
func (r *LLMRouterV2) refreshCaches() {
    // ...
    for name := range r.config.LLM.Providers {
        pcfg := r.config.LLM.Providers[name]
        if !pcfg.Enabled {
            continue  // ← УЖЕ должно работать, проверить!
        }
        // ...
        if r.config.VoiceMessages.Enabled {
            if ag, ok := provider.(llm.AudioGenerator); ok {
                r.audioGens = append(r.audioGens, ag)
            }
        }
        if r.config.FreeWill.ImageGeneration.MaxPerInterval > 0 {
            if ig, ok := provider.(llm.ImageGenerator); ok {
                r.imageGens = append(r.imageGens, ig)
            }
        }
        // ImageAnalyzer — всегда регистрируем (нужен для photo_analysis)
    }
}
```

### 15.8 План миграции

**Шаг 1: Конфиг (config.go + YAML)**
- Добавить `RequestTimeoutSeconds`, `RetryConfig` в `ProviderConfig`
- Прописать значения в YAML для local и gemini
- Упростить `response_types` (удалить дубликаты и post_process_*)
- Почистить `fallback_provider_order` и fallback-ссылки

**Шаг 2: ProviderManager (llm_router_v2.go)**
- Реализовать `tryWithRetry()` с context timeout + retry loop + backoff
- Улучшить `isRetryableError()` (добавить `context.DeadlineExceeded`, `net.Error`)
- Интегрировать KeyRotation в retry loop (при 429 → rotate → retry)

**Шаг 3: Очистка клиентов**
- `local/client.go`: удалить `doChatCompletionWithRetry`, `isTimeoutError`
- `local/client.go`: убрать `context.WithTimeout` из `doChatCompletion`
- `gemini/client.go`: заменить `http.Client{Timeout: 30s}` на значение из конфига (safety net)
- Остальные клиенты: аналогично убрать дублирующие таймауты

**Шаг 4: Блокировка не-текстовых capability**
- `refreshCaches()`: проверять `VoiceMessages.Enabled`, `ImageGeneration.MaxPerInterval`
- `refreshCaches()`: проверять `pcfg.Enabled` перед Resolve

**Шаг 5: Тесты**
- Unit-тесты для `tryWithRetry` (timeout, retry, backoff, CB)
- Интеграционный тест: primary fail → fallback success
- Тест: не-retryable ошибка не триггерит fallback

### 15.9 Файлы к изменению

| # | Файл | Изменения |
|---|------|-----------|
| 1 | `configs/luna_bot.yaml` | +`request_timeout_seconds`, +`retry` для providers; упростить response_types (54→~25); убрать dead fallback ссылки; `voice_messages.enabled: false`; `tts.provider: ""` |
| 2 | `internal/config/config.go` | +`RequestTimeoutSeconds` и `RetryConfig` в `ProviderConfig` |
| 3 | `internal/bot/llm_router_v2.go` | +`tryWithRetry()`, улучшить `isRetryableError()`, `refreshCaches()` — проверка `enabled`, `VoiceMessages`, `ImageGeneration` |
| 4 | `internal/local/client.go` | Удалить `doChatCompletionWithRetry`, `isTimeoutError`; убрать `context.WithTimeout` из `doChatCompletion` |
| 5 | `internal/gemini/client.go` | `http.Client{Timeout}` — брать из конфига; удалить дублирующую retry-логику key rotation (перенести в router) |
| 6 | `internal/deepseek/client.go` | Убрать хардкод-таймауты (если есть) |
| 7 | `internal/openrouter/client.go` | Убрать `context.WithTimeout` (150s), оставить только HTTP client timeout |
| 8 | `internal/openai/client.go` | Убрать `context.WithTimeout` (150s), оставить только HTTP client timeout |
| 9 | `internal/anthropic/client.go` | Убрать `context.WithTimeout` (150s), оставить только HTTP client timeout |
