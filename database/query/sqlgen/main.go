// SPDX-License-Identifier: ice License 1.0

//go:build test

package main

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func FormatSQLWithParams(sqlQuery string, params map[string]any) string {
	result := sqlQuery

	// Handle both :param and $param styles.
	paramRegex := regexp.MustCompile(`[:$]([a-zA-Z0-9_.]+)`)

	matches := paramRegex.FindAllStringSubmatch(result, -1)
	slices.SortStableFunc(matches, func(a, b []string) int {
		// Sort by length of the parameter name in descending order.
		if len(a[1]) > len(b[1]) {
			return -1
		} else if len(a[1]) < len(b[1]) {
			return 1
		}
		return 0
	})

	for _, match := range matches {
		placeholder := match[0]
		paramName := match[1]

		value, exists := params[paramName]
		if !exists {
			continue
		}

		var formattedValue string

		switch v := value.(type) {
		case string:
			// Escape single quotes and wrap in quotes.
			escaped := strings.ReplaceAll(v, "'", "''")
			formattedValue = "'" + escaped + "'"
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			formattedValue = fmt.Sprintf("%d", v)
		case float32, float64:
			formattedValue = fmt.Sprintf("%f", v)
		case bool:
			formattedValue = fmt.Sprintf("%t", v)
		case time.Time:
			formattedValue = "'" + v.Format(time.RFC3339) + "'"
		case nil:
			formattedValue = "NULL"
		default:
			// Handle slices and arrays
			if reflect.TypeOf(value).Kind() == reflect.Slice {
				formattedValue = formatArrayValue(value)
			} else {
				// Default fallback
				formattedValue = fmt.Sprintf("'%v'", v)
			}
		}

		// Replace the parameter.
		result = strings.ReplaceAll(result, placeholder, formattedValue)
	}

	return result
}

func formatArrayValue(value any) string {
	val := reflect.ValueOf(value)
	length := val.Len()
	elements := make([]string, length)

	for i := 0; i < length; i++ {
		elem := val.Index(i).Interface()
		switch e := elem.(type) {
		case string:
			escaped := strings.ReplaceAll(e, "'", "''")
			elements[i] = "'" + escaped + "'"
		default:
			elements[i] = fmt.Sprintf("%v", e)
		}
	}

	return "ARRAY[" + strings.Join(elements, ", ") + "]"
}

func main() {
	var dataContext model.UserDataContext

	masterKey := flag.String("master-key", "", "Master private key in hex format")
	publicKey := flag.String("public-key", "", "Public key in hex format")
	printFilter := flag.Bool("debug", false, "Print filter as JSON")

	flag.Parse()

	dataContext.Authenticated = len(cmp.Or(*masterKey, *publicKey)) > 0
	dataContext.MasterPublicKey = cmp.Or(*masterKey, *publicKey)
	dataContext.PublicKey = *publicKey

	if dataContext.Authenticated {
		fmt.Printf("Using authenticated context with public key: %s and master public key: %s\n", dataContext.PublicKey, dataContext.MasterPublicKey)
	}
	ctx := model.SetUserDataInContext(context.Background(), dataContext)

	for _, a := range flag.Args() {
		var f model.Filters

		if r, err := base64.StdEncoding.DecodeString(a); err == nil {
			a = string(r)
			if *printFilter {
				fmt.Printf("Decoded base64 filter: %s\n", a)
			}
		}

		err := json.Unmarshal([]byte(a), &f)
		if err != nil {
			panic("failed to unmarshal filters: " + err.Error())
		}

		sql, params, err := query.GenerateSelectEventsSQL(ctx, f...)
		if err != nil {
			panic("failed to generate SQL: " + err.Error())
		}
		fmt.Println(FormatSQLWithParams(sql, params))
	}
}
