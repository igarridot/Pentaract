package repository

import (
	"strings"
)

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "unique constraint")
}
