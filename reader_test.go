package lzo

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func Test_Decompression_Fixed(t *testing.T) {

	setupTestEnvironment(t)

	cases := []struct {
		Name string
		Data []byte
	}{
		{
			Name: "empty",
			Data: []byte{},
		},
		{
			Name: "really-short",
			Data: []byte("Short stack!"),
		},
		{
			Name: "short",
			Data: []byte("Hello, World! This is a small test string."),
		},
		{
			Name: "short-repeated",
			Data: bytes.Repeat([]byte("ABCD"), 100),
		},
		{
			Name: "long-repeated",
			Data: bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 200),
		},
		{
			Name: "zeros",
			Data: make([]byte, 1000),
		},
		{
			Name: "lorem-ipsum",
			Data: []byte(`Lorem ipsum dolor sit amet, consectetur adipiscing elit. 
Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. 
Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris 
nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in 
reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla 
pariatur. Excepteur sint occaecat cupidatat non proident, sunt in 
culpa qui officia deserunt mollit anim id est laborum.`),
		},
		{
			Name: "binary",
			Data: func() []byte {
				data := make([]byte, 256)
				for i := range data {
					data[i] = byte(i)
				}
				return data
			}(),
		},
		{
			Name: "mixed",
			Data: func() []byte {
				var buf bytes.Buffer
				buf.WriteString("Text section with repeating patterns.\n")
				buf.Write(bytes.Repeat([]byte{0xAA, 0xBB}, 50))
				buf.WriteString("\nMore text here with different content.\n")
				buf.Write(bytes.Repeat([]byte{0x00}, 100))
				buf.WriteString("\nFinal text section.\n")
				return buf.Bytes()
			}(),
		},
		{
			Name: "json",
			Data: []byte(`{
  "users": [
    {"id": 1, "name": "Alice", "email": "alice@example.com"},
    {"id": 2, "name": "Bob", "email": "bob@example.com"},
    {"id": 3, "name": "Charlie", "email": "charlie@example.com"}
  ],
  "metadata": {
    "version": "1.0",
    "created": "2024-01-01",
    "count": 3
  }
}`),
		},
	}

	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			vs := compressFixture(t, tt.Name, tt.Data)
			for _, v := range vs {
				t.Run(v.Method, func(t *testing.T) {
					reader := NewReader(bytes.NewReader(v.Compressed))
					decompressed, err := io.ReadAll(reader)
					if err != nil {
						t.Fatalf("decompression failed: %v", err)
					}

					if !bytes.Equal(decompressed, tt.Data) {
						t.Errorf("decompressed data doesn't match original")
						t.Errorf("original length    : %d", len(tt.Data))
						t.Errorf("decompressed length: %d", len(decompressed))
					}
				})
			}
		})
	}
}

func Test_Decompression_Generated(t *testing.T) {

	setupTestEnvironment(t)
	v := cValidator(t)

	for _, size := range testSizes {
		for _, pattern := range testPatterns {
			t.Run(fmt.Sprintf("%s_%dB", pattern, size), func(t *testing.T) {
				og := generateTestData(t, size, pattern)

				d := t.TempDir()
				inputFile := fmt.Sprintf("%s/original.bin", d)
				if err := os.WriteFile(inputFile, og, 0644); err != nil {
					t.Fatalf("Failed to write original data: %v", err)
				}

				// use golden file to compare against...
				compressed := v.Compress(t, inputFile)

				dst1 := make([]byte, len(og)*2)

				// subject under test...
				size1, err1 := Decompress(compressed, dst1)
				if err1 != nil {
					t.Fatalf("Original decompression failed: %v", err1)
				}

				// check sizes match
				if size1 != len(og) {
					t.Errorf("Size mismatch: original=%d, candidate=%d", size1, len(og))
				}

				// verify decompressed data matches original
				if !bytes.Equal(dst1[:size1], og) {
					saveTestCase(t, og)
					t.Errorf("Decompressed data doesn't match original for %s pattern", pattern)
				}
			})
		}
	}
}

func TestLZOEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
	}{
		{
			name:        "too_short_input",
			data:        []byte{0x00, 0x01},
			expectError: false, // this isn't an error case explicitly
		},
		{
			name:        "invalid_instruction",
			data:        []byte{0x00, 0x00, 0x00},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewReader(bytes.NewReader(tt.data))
			contents, err := io.ReadAll(reader)

			if len(contents) > 0 {
				t.Errorf("Expected no output, but got %d bytes", len(contents))
			}
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func BenchmarkDecompress(b *testing.B) {

	setupTestEnvironment(b)
	v := cValidator(b)

	for _, size := range testSizes {
		for _, pattern := range testPatterns {
			name := fmt.Sprintf("%s_%dKB", pattern, size/1024)
			b.Run(name, func(b *testing.B) {
				og := generateTestData(b, size, pattern)

				dir := b.TempDir()
				inputFile := fmt.Sprintf("%s/%s_%d.bin", dir, pattern, size)
				if err := os.WriteFile(inputFile, og, 0644); err != nil {
					b.Fatalf("Failed to write original data: %v", err)
				}

				compressed := v.Compress(b, inputFile)
				dst := make([]byte, len(og)*2) // ensure enough space

				b.ResetTimer()
				b.SetBytes(int64(len(compressed)))

				for i := 0; i < b.N; i++ {
					_, err := Decompress(compressed, dst)
					if err != nil {
						b.Fatalf("Decompression failed: %v", err)
					}
				}
			})
		}
	}
}

func BenchmarkMemoryAllocations(b *testing.B) {

	setupTestEnvironment(b)
	v := cValidator(b)
	og := generateTestData(b, 64*1024, "mixed")

	tempDir := b.TempDir()
	inputFile := fmt.Sprintf("%s/original.bin", tempDir)
	if err := os.WriteFile(inputFile, og, 0644); err != nil {
		b.Fatalf("Failed to write original data: %v", err)
	}

	compressed := v.Compress(b, inputFile)

	dst := make([]byte, len(og)*2)

	b.Run("Original", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			Decompress(compressed, dst)
		}
	})

}

func FuzzCompressDecompress(f *testing.F) {

	setupTestEnvironment(f)
	v := cValidator(f)

	for _, size := range testSizes {
		for _, pattern := range testPatterns {
			f.Add(generateTestData(f, size, pattern))
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		inputFile := filepath.Join(dir, "input.bin")
		err := os.WriteFile(inputFile, data, 0644)
		if err != nil {
			t.Fatalf("Failed to write input file: %v", err)
		}

		compressed := v.Compress(t, inputFile)

		dst := make([]byte, len(data)*2)
		n, err := Decompress(compressed, dst)
		if err != nil {
			saveTestCase(t, data)
			t.Fatalf("Decompression failed: %v", err)
		}
		if !bytes.Equal(dst[:n], data) {
			saveTestCase(t, data)
			t.Errorf("Decompressed data does not match original data")
		}
	})
}
