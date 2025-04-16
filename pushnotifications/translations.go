// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type (
	Language           string
	TranslationManager struct {
		mu           sync.RWMutex
		translations map[NotificationType]map[Language]map[string]string
	}
)

func NewTranslationManager(translationsDir string) *TranslationManager {
	tm := &TranslationManager{
		translations: make(map[NotificationType]map[Language]map[string]string),
	}

	if translationsDir != "" {
		if err := tm.loadTranslations(translationsDir); err != nil {
			panic(err)
		}
	} else {
		panic("translations directory is not set")
	}

	return tm
}

func (tm *TranslationManager) loadTranslations(translationsDir string) error {
	files, err := os.ReadDir(translationsDir)
	if err != nil {
		return fmt.Errorf("failed to read translations directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		key := strings.TrimSuffix(file.Name(), ".json")
		filePath := filepath.Join(translationsDir, file.Name())

		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("warning: failed to read translation file %s: %v", filePath, err)

			continue
		}

		var translations map[NotificationType]map[string]string
		if err := json.Unmarshal(content, &translations); err != nil {
			log.Printf("warning: failed to parse translation file %s: %v", filePath, err)

			continue
		}

		tm.mu.Lock()
		for lang, trans := range translations {
			if _, ok := tm.translations[NotificationType(key)]; !ok {
				tm.translations[NotificationType(key)] = make(map[Language]map[string]string)
			}
			tm.translations[NotificationType(key)][Language(lang)] = trans
		}
		tm.mu.Unlock()
	}

	return nil
}

func (tm *TranslationManager) GetTranslation(notificationType NotificationType, language Language, key string, placeholders map[string]interface{}) string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	langMap, ok := tm.translations[notificationType]
	if !ok {
		return fmt.Sprintf("%v_%v", language, key)
	}

	keyMap, ok := langMap[language]
	if !ok {
		if englishMap, exists := langMap["en"]; exists {
			keyMap = englishMap
		} else {
			return fmt.Sprintf("%v_%v", language, key)
		}
	}

	translation, ok := keyMap[key]
	if !ok {
		return fmt.Sprintf("%v_%v", language, key)
	}

	if placeholders != nil {
		for placeholder, value := range placeholders {
			if value == nil {
				continue
			}
			strValue := fmt.Sprintf("%v", value)
			translation = strings.Replace(translation, "{{"+placeholder+"}}", strValue, -1)
		}
	}

	return translation
}

func (tm *TranslationManager) GetAvailableLanguages() []Language {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	languagesMap := make(map[Language]bool)

	for _, langMap := range tm.translations {
		for lang := range langMap {
			languagesMap[lang] = true
		}
	}

	languages := make([]Language, 0, len(languagesMap))
	for lang := range languagesMap {
		languages = append(languages, lang)
	}

	return languages
}
