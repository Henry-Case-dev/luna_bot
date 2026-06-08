package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// GetPromptDir возвращает путь к директории с промптами
// на основе расположения этого файла.
func GetPromptDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

// LoadPrompt загружает промпт из директории промптов.
//
// Алгоритм поиска (в порядке приоритета):
//  1. Файл <name>.txt — если существует, возвращается его полное содержимое.
//  2. Секция ">>> <name>" в любом .txt файле — если найдена, возвращается
//     текст секции (всё от заголовка до следующего ">>> " или конца файла).
//
// Если промпт не найден или файл пуст, возвращает пустую строку.
func LoadPrompt(name string) (string, error) {
	promptDir := GetPromptDir()

	// Шаг 1: Пробуем файл <name>.txt (старый формат)
	filePath := filepath.Join(promptDir, name+".txt")
	data, err := os.ReadFile(filePath)
	if err == nil {
		content := strings.TrimSpace(string(data))
		if content != "" {
			return content, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("ошибка чтения файла промпта %s: %w", filePath, err)
	}

	// Шаг 2: Ищем секцию ">>> <name>" во всех .txt файлах (новый модульный формат)
	entries, err := os.ReadDir(promptDir)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения директории промптов %s: %w", promptDir, err)
	}

	sectionHeader := ">>> " + name
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		fileData, err := os.ReadFile(filepath.Join(promptDir, entry.Name()))
		if err != nil {
			continue
		}

		content := string(fileData)
		if idx := strings.Index(content, sectionHeader); idx != -1 {
			// Нашли секцию — извлекаем содержимое после ">>> name\n"
			sectionStart := idx + len(sectionHeader)
			// Пропускаем возможный перевод строки после заголовка
			if sectionStart < len(content) && content[sectionStart] == '\n' {
				sectionStart++
			} else if sectionStart+1 < len(content) && content[sectionStart:sectionStart+2] == "\r\n" {
				sectionStart += 2
			}

			// Ищем следующую секцию ">>> " или конец файла
			rest := content[sectionStart:]
			nextSectionIdx := strings.Index(rest, "\n>>> ")
			if nextSectionIdx == -1 {
				// Может быть в начале строки (без \n перед)
				nextSectionIdx = strings.Index(rest, ">>> ")
			}

			var sectionContent string
			if nextSectionIdx != -1 {
				sectionContent = rest[:nextSectionIdx]
			} else {
				sectionContent = rest
			}

			sectionContent = strings.TrimSpace(sectionContent)
			if sectionContent != "" {
				return sectionContent, nil
			}
		}
	}

	return "", nil
}

// LoadPromptWithDefault загружает промпт из файла, а если его нет —
// возвращает значение по умолчанию.
func LoadPromptWithDefault(name string, defaultVal string) string {
	content, err := LoadPrompt(name)
	if err != nil || content == "" {
		return defaultVal
	}
	return content
}

// LoadPromptOrEnv загружает промпт: сначала из файла (если не пустой),
// затем из env (если не пустой), затем defaultVal.
//
// Приоритет: файл > env > default
func LoadPromptOrEnv(name, envVal, defaultVal string) string {
	// Сначала пробуем файл
	fileVal, err := LoadPrompt(name)
	if err == nil && fileVal != "" {
		return fileVal
	}
	// Затем env
	if envVal != "" {
		return envVal
	}
	// Затем default
	return defaultVal
}

// GetPromptFilenames возвращает список всех .txt файлов в директории промптов.
func GetPromptFilenames() ([]string, error) {
	promptDir := GetPromptDir()
	entries, err := os.ReadDir(promptDir)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения директории промптов %s: %w", promptDir, err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txt") {
			names = append(names, strings.TrimSuffix(entry.Name(), ".txt"))
		}
	}
	return names, nil
}
