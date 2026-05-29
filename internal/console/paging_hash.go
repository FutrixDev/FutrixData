package console

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func pagingQueryHash(statement string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(statement)), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
