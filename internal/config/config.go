package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

var Logger = newLogger()

func newLogger() *slog.Logger {
	level := new(slog.LevelVar)
	switch strings.ToUpper(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "DEBUG":
		level.Set(slog.LevelDebug)
	case "WARN":
		level.Set(slog.LevelWarn)
	case "ERROR":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}

type Account struct {
	Email    string `json:"email,omitempty"`
	Mobile   string `json:"mobile,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

func (a Account) Identifier() string {
	if strings.TrimSpace(a.Email) != "" {
		return strings.TrimSpace(a.Email)
	}
	return strings.TrimSpace(a.Mobile)
}

type Config struct {
	Keys             []string          `json:"keys,omitempty"`
	Accounts         []Account         `json:"accounts,omitempty"`
	ClaudeMapping    map[string]string `json:"claude_mapping,omitempty"`
	ClaudeModelMap   map[string]string `json:"claude_model_mapping,omitempty"`
	VercelSyncHash   string            `json:"_vercel_sync_hash,omitempty"`
	VercelSyncTime   int64             `json:"_vercel_sync_time,omitempty"`
	AdditionalFields map[string]any    `json:"-"`
}

func (c Config) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range c.AdditionalFields {
		m[k] = v
	}
	if len(c.Keys) > 0 {
		m["keys"] = c.Keys
	}
	if len(c.Accounts) > 0 {
		m["accounts"] = c.Accounts
	}
	if len(c.ClaudeMapping) > 0 {
		m["claude_mapping"] = c.ClaudeMapping
	}
	if len(c.ClaudeModelMap) > 0 {
		m["claude_model_mapping"] = c.ClaudeModelMap
	}
	if c.VercelSyncHash != "" {
		m["_vercel_sync_hash"] = c.VercelSyncHash
	}
	if c.VercelSyncTime != 0 {
		m["_vercel_sync_time"] = c.VercelSyncTime
	}
	return json.Marshal(m)
}

func (c *Config) UnmarshalJSON(b []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.AdditionalFields = map[string]any{}
	for k, v := range raw {
		switch k {
		case "keys":
			_ = json.Unmarshal(v, &c.Keys)
		case "accounts":
			_ = json.Unmarshal(v, &c.Accounts)
		case "claude_mapping":
			_ = json.Unmarshal(v, &c.ClaudeMapping)
		case "claude_model_mapping":
			_ = json.Unmarshal(v, &c.ClaudeModelMap)
		case "_vercel_sync_hash":
			_ = json.Unmarshal(v, &c.VercelSyncHash)
		case "_vercel_sync_time":
			_ = json.Unmarshal(v, &c.VercelSyncTime)
		default:
			var anyVal any
			if err := json.Unmarshal(v, &anyVal); err == nil {
				c.AdditionalFields[k] = anyVal
			}
		}
	}
	return nil
}

func (c Config) Clone() Config {
	clone := Config{
		Keys:             slices.Clone(c.Keys),
		Accounts:         slices.Clone(c.Accounts),
		ClaudeMapping:    cloneStringMap(c.ClaudeMapping),
		ClaudeModelMap:   cloneStringMap(c.ClaudeModelMap),
		VercelSyncHash:   c.VercelSyncHash,
		VercelSyncTime:   c.VercelSyncTime,
		AdditionalFields: map[string]any{},
	}
	for k, v := range c.AdditionalFields {
		clone.AdditionalFields[k] = v
	}
	return clone
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type Store struct {
	mu      sync.RWMutex
	cfg     Config
	path    string
	fromEnv bool
}

func BaseDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func IsVercel() bool {
	return strings.TrimSpace(os.Getenv("VERCEL")) != "" || strings.TrimSpace(os.Getenv("NOW_REGION")) != ""
}

func ResolvePath(envKey, defaultRel string) string {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw != "" {
		if filepath.IsAbs(raw) {
			return raw
		}
		return filepath.Join(BaseDir(), raw)
	}
	return filepath.Join(BaseDir(), defaultRel)
}

func ConfigPath() string {
	return ResolvePath("DS2API_CONFIG_PATH", "config.json")
}

func WASMPath() string {
	return ResolvePath("DS2API_WASM_PATH", "sha3_wasm_bg.7b9ca65ddd.wasm")
}

func StaticAdminDir() string {
	return ResolvePath("DS2API_STATIC_ADMIN_DIR", "static/admin")
}

func LoadStore() *Store {
	cfg, fromEnv, err := loadConfig()
	if err != nil {
		Logger.Warn("[config] load failed", "error", err)
	}
	if len(cfg.Keys) == 0 && len(cfg.Accounts) == 0 {
		Logger.Warn("[config] empty config loaded")
	}
	return &Store{cfg: cfg, path: ConfigPath(), fromEnv: fromEnv}
}

func loadConfig() (Config, bool, error) {
	rawCfg := strings.TrimSpace(os.Getenv("DS2API_CONFIG_JSON"))
	if rawCfg == "" {
		rawCfg = strings.TrimSpace(os.Getenv("CONFIG_JSON"))
	}
	if rawCfg != "" {
		cfg, err := parseConfigString(rawCfg)
		return cfg, true, err
	}

	content, err := os.ReadFile(ConfigPath())
	if err != nil {
		return Config{}, false, err
	}
	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, false, err
	}
	return cfg, false, nil
}

func parseConfigString(raw string) (Config, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err == nil {
		return cfg, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(decoded, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

func (s *Store) HasAPIKey(k string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, key := range s.cfg.Keys {
		if key == k {
			return true
		}
	}
	return false
}

func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Keys)
}

func (s *Store) Accounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Accounts)
}

func (s *Store) FindAccount(identifier string) (Account, bool) {
	identifier = strings.TrimSpace(identifier)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, acc := range s.cfg.Accounts {
		if acc.Identifier() == identifier {
			return acc, true
		}
	}
	return Account{}, false
}

func (s *Store) UpdateAccountToken(identifier, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Accounts {
		if s.cfg.Accounts[i].Identifier() == identifier {
			s.cfg.Accounts[i].Token = token
			return s.saveLocked()
		}
	}
	return errors.New("account not found")
}

func (s *Store) Replace(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg.Clone()
	return s.saveLocked()
}

func (s *Store) Update(mutator func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg.Clone()
	if err := mutator(&cfg); err != nil {
		return err
	}
	s.cfg = cfg
	return s.saveLocked()
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.fromEnv {
		Logger.Info("[save_config] source from env, skip write")
		return nil
	}
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func (s *Store) saveLocked() error {
	if s.fromEnv {
		Logger.Info("[save_config] source from env, skip write")
		return nil
	}
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func (s *Store) IsEnvBacked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fromEnv
}

func (s *Store) SetVercelSync(hash string, ts int64) error {
	return s.Update(func(c *Config) error {
		c.VercelSyncHash = hash
		c.VercelSyncTime = ts
		return nil
	})
}

func (s *Store) ExportJSONAndBase64() (string, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := json.Marshal(s.cfg)
	if err != nil {
		return "", "", err
	}
	return string(b), base64.StdEncoding.EncodeToString(b), nil
}

func (s *Store) ClaudeMapping() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.cfg.ClaudeModelMap) > 0 {
		return cloneStringMap(s.cfg.ClaudeModelMap)
	}
	if len(s.cfg.ClaudeMapping) > 0 {
		return cloneStringMap(s.cfg.ClaudeMapping)
	}
	return map[string]string{"fast": "deepseek-chat", "slow": "deepseek-chat"}
}
