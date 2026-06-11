package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigChange — уведомление об изменении конфигурации.
type ConfigChange struct {
	Path string      // YAML-путь изменившегося поля, например "llm.default_provider"
	Old  interface{} // Старое значение
	New  interface{} // Новое значение
}

// ConfigSource — абстрактный источник конфигурации.
type ConfigSource interface {
	// Load загружает конфигурацию.
	Load(ctx context.Context) (*ConfigV2, error)

	// Watch возвращает канал уведомлений об изменениях конфигурации.
	// Если источник не поддерживает hot-reload, возвращает nil.
	Watch(ctx context.Context) (<-chan ConfigChange, error)
}

// YAMLConfigSource загружает конфигурацию из YAML-файла.
type YAMLConfigSource struct {
	path       string // Путь к YAML-файлу
	envPrefix  string // Префикс для env-переопределений (по умолчанию "LUNA_")
	strictMode bool   // Ошибка при неизвестных ключах
}

// NewYAMLConfigSource создаёт новый YAMLConfigSource.
func NewYAMLConfigSource(path string) *YAMLConfigSource {
	return &YAMLConfigSource{
		path:       path,
		envPrefix:  "LUNA_",
		strictMode: true,
	}
}

// SetEnvPrefix задаёт префикс для env-переопределений.
func (s *YAMLConfigSource) SetEnvPrefix(prefix string) {
	s.envPrefix = prefix
}

// SetStrictMode задаёт строгий режим парсинга.
func (s *YAMLConfigSource) SetStrictMode(strict bool) {
	s.strictMode = strict
}

// Load загружает конфигурацию из YAML-файла.
func (s *YAMLConfigSource) Load(ctx context.Context) (*ConfigV2, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", s.path, err)
	}

	content := resolveEnvVars(string(data))

	var cfg ConfigV2
	decoder := yaml.NewDecoder(strings.NewReader(content))
	if s.strictMode {
		decoder.KnownFields(true)
	}
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Валидация конфигурации
	if errs := ValidateConfigV2(&cfg); len(errs) > 0 {
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return nil, fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(msgs, "\n  - "))
	}

	applyEnvOverrides(&cfg, s.envPrefix)

	return &cfg, nil
}

// Watch возвращает канал уведомлений об изменениях конфигурации.
// YAMLConfigSource пока не поддерживает hot-reload.
func (s *YAMLConfigSource) Watch(ctx context.Context) (<-chan ConfigChange, error) {
	return nil, nil
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// resolveEnvVars заменяет ${VAR} на значение из os.Getenv("VAR").
func resolveEnvVars(yamlContent string) string {
	return envVarPattern.ReplaceAllStringFunc(yamlContent, func(match string) string {
		varName := match[2 : len(match)-1]
		return os.Getenv(varName)
	})
}

// applyEnvOverrides применяет переопределения из env-переменных.
// Формат: LUNA_LLM__GEMINI__API_KEY → путь llm.gemini.api_key → значение.
func applyEnvOverrides(cfg *ConfigV2, envPrefix string) {
	prefix := strings.ToUpper(envPrefix)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]

		if !strings.HasPrefix(strings.ToUpper(key), prefix) || len(key) <= len(prefix) {
			continue
		}

		pathStr := key[len(prefix):]
		segments := strings.Split(pathStr, "__")
		var path []string
		for _, s := range segments {
			s = strings.TrimSpace(s)
			if s != "" {
				path = append(path, strings.ToLower(s))
			}
		}

		if len(path) == 0 {
			continue
		}

		if err := setNestedField(cfg, path, value); err != nil {
			log.Printf("[WARN] Failed to apply env override %s: %v", key, err)
		}
	}
}

// setNestedField устанавливает вложенное поле структуры через рефлексию.
// path — слайс сегментов пути (например, ["llm", "gemini", "api_key"]).
func setNestedField(cfg interface{}, path []string, value string) error {
	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	for i, segment := range path {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		found := false

		if v.Kind() == reflect.Struct {
			for j := 0; j < v.NumField(); j++ {
				field := v.Type().Field(j)
				if getYamlTag(field) == segment {
					v = v.Field(j)
					found = true
					break
				}
			}
		}

		if !found && v.Kind() == reflect.Struct {
			for j := 0; j < v.NumField(); j++ {
				field := v.Field(j)
				if field.Kind() == reflect.Map && field.Type().Key().Kind() == reflect.String {
					key := reflect.ValueOf(segment)
					mapVal := field.MapIndex(key)
					if !mapVal.IsValid() {
						mapVal = reflect.New(field.Type().Elem()).Elem()
						field.SetMapIndex(key, mapVal)
					}
					v = mapVal
					found = true
					break
				}
			}
		}

		if !found {
			return fmt.Errorf("field not found in path: %s (at segment: %s)", strings.Join(path, "."), segment)
		}

		if i == len(path)-1 {
			return setFieldValue(v, value)
		}
	}

	return nil
}

// getYamlTag извлекает имя yaml-тега из поля структуры.
func getYamlTag(field reflect.StructField) string {
	tag := field.Tag.Get("yaml")
	if tag == "" {
		return ""
	}
	parts := strings.Split(tag, ",")
	return parts[0]
}

// setFieldValue устанавливает значение поля с конвертацией типа.
func setFieldValue(v reflect.Value, value string) error {
	if !v.CanSet() {
		return fmt.Errorf("cannot set field")
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(value)
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value %q: %w", value, err)
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid duration value %q: %w", value, err)
			}
			v.SetInt(int64(d))
		} else {
			i, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid int value %q: %w", value, err)
			}
			v.SetInt(i)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid uint value %q: %w", value, err)
		}
		v.SetUint(i)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid float value %q: %w", value, err)
		}
		v.SetFloat(f)
	default:
		return fmt.Errorf("unsupported type: %s", v.Kind())
	}
	return nil
}
