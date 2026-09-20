package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func detectPEM(path, content string) []SecretFinding {
	lines := strings.SplitAfter(content, "\n")
	var findings []SecretFinding
	offset := 0
	for index, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r\n")
		label, isHeader := pemHeader(line)
		if isHeader {
			endOffset := offset + len(rawLine)
			for endLine := index + 1; endLine < len(lines); endLine++ {
				endOffset += len(lines[endLine])
				if strings.TrimRight(lines[endLine], "\r\n") == "-----END "+label+"-----" {
					break
				}
			}
			if endOffset > len(content) {
				endOffset = len(content)
			}
			findings = append(findings, SecretFinding{
				Type:   "pem",
				Path:   path,
				Line:   index + 1,
				SHA256: hashContent(content[offset:endOffset]),
			})
		}
		offset += len(rawLine)
	}
	return findings
}

func pemHeader(line string) (string, bool) {
	const prefix = "-----BEGIN "
	const suffix = "-----"
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) || len(line) <= len(prefix)+len(suffix) {
		return "", false
	}
	label := strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix)
	if label == "" || strings.TrimSpace(label) != label {
		return "", false
	}
	for _, char := range label {
		if !((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == ' ' || char == '-') {
			return "", false
		}
	}
	return label, true
}

func hashContent(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
