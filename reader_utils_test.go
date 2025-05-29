package lzo

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	cacheDir     = "testdata/cache"
	dockerFile   = "testdata/Dockerfile"
	dockerImage  = "go-lzo-fixture"
	containerDir = "/workspace"
	cToolPath    = "testdata/bin/lzo-tool"
)

// test data sizes (in bytes)
var testSizes = []int{
	1024,        // 1KB
	16 * 1024,   // 16KB
	64 * 1024,   // 64KB
	256 * 1024,  // 256KB
	1024 * 1024, // 1MB
}

// test data patterns
var testPatterns = []string{
	"random",
	"repeated",
	"zeros",
	"text",
	"mixed",
}

type validator struct {
	Method     string
	Compress   func(t testing.TB, inputFile string) []byte
	Decompress func(t testing.TB, inputFile string) []byte
	Compressed []byte
}

func (v validator) Path(name string) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%s.%s.lzo", name, v.Method))
}

func setupTestEnvironment(t testing.TB) {
	t.Helper()

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache directory: %v", err)
	}

	if _, err := os.Stat(dockerFile); os.IsNotExist(err) {
		t.Fatalf("dockerfile not found: %s", dockerFile)
	}

	if !attemptToBuildCValidator(t) {
		buildDockerImage(t)
	}

}

func attemptToBuildCValidator(t testing.TB) bool {
	// TODO: build the c validator if it does not exist... place into the testdata/bin directory as lzo-tool
	if _, err := os.Stat(cToolPath); !os.IsNotExist(err) {
		t.Log("C validator already built")
		return true
	}

	parentDir := filepath.Dir(cToolPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		t.Fatalf("failed to create directory for C validator: %v", err)
	}

	args := []string{"-o", cToolPath, "testdata/lzo-tool.cpp", "-llzo2"}

	if strings.Contains(strings.ToLower(os.Getenv("GOOS")), "darwin") {
		args = append(args, "-I/opt/homebrew/include", "-L/opt/homebrew/lib")
	}

	// run: g++ -o testdata/bin/lzo-tool lzo-tool.cpp -llzo2
	cmd := exec.Command("g++", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// this should not be fatal, but we should log it...
		t.Logf("failed to build C validator: %v\nOutput: %s", err, output)
		return false
	}
	return true
}

func buildDockerImage(t testing.TB) {
	t.Helper()

	// TODO: build and inspect based off of the dockerfile hash + the remaining docker context
	cmd := exec.Command("docker", "images", "-q", dockerImage)
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		t.Logf("docker image %s already exists", dockerImage)
		return
	}

	cmd = exec.Command("docker", "build", "-t", dockerImage, "-f", dockerFile, ".")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build Docker image: %v\nOutput: %s", err, output)
	}
	t.Logf("docker image %s built successfully", dockerImage)
}

func cValidator(t testing.TB) validator {
	// use native binary if available (faster but harder to universally incorporate), otherwise use Docker (slower but more portable)
	if _, err := os.Stat(cToolPath); err == nil {
		t.Logf("using native LZO validator...")
		return cValidatorViaNaive()
	}

	return cValidatorViaDocker()
}

func cValidatorViaNaive() validator {
	return validator{
		Method: "cpp",
		Compress: func(t testing.TB, inputPath string) []byte {
			cmd := exec.Command("testdata/bin/lzo-tool", "-c", inputPath)

			compressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("lzo-wrapper compression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run lzo-wrapper: %v", err)
			}
			return compressed
		},
		Decompress: func(t testing.TB, inputPath string) []byte {
			cmd := exec.Command("testdata/bin/lzo-tool", "-d", inputPath)

			decompressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("lzo-wrapper decompression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run lzo-wrapper decompression: %v", err)
			}
			return decompressed
		},
	}
}

func cValidatorViaDocker() validator {
	return validator{
		Method: "cpp",
		Compress: func(t testing.TB, inputPath string) []byte {
			parent := filepath.Dir(inputPath)
			filename := filepath.Base(inputPath)

			cmd := exec.Command("docker", "run", "--rm",
				"-v", fmt.Sprintf("%s:%s", parent, containerDir),
				dockerImage,
				"lzo-tool", "-c", filepath.Join(containerDir, filename))

			compressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("lzo-wrapper compression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run lzo-wrapper: %v", err)
			}
			return compressed
		},
		Decompress: func(t testing.TB, inputPath string) []byte {
			parent := filepath.Dir(inputPath)
			filename := filepath.Base(inputPath)

			cmd := exec.Command("docker", "run", "--rm",
				"-v", fmt.Sprintf("%s:%s", parent, containerDir),
				dockerImage,
				"lzo-tool", "-d", filepath.Join(containerDir, filename))

			decompressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("lzo-wrapper decompression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run lzo-wrapper decompression: %v", err)
			}
			return decompressed
		},
	}
}

func validators(t testing.TB) []validator {
	return []validator{
		// simply an lzo raw stream, no headers or framing
		cValidator(t),

		// this adds an 8 byte header with size information
		//cWithHeaderValidator(),

		// this appears to add a variable length header with information, so isn't easy to use
		//pythonLzoValidator(),

		// additional lzop validator can be added that handles lzop framing...
		//lzopValidator(t),
	}
}

func cWithHeaderValidator() validator {
	// this adds an 8 byte header with size information
	return validator{
		Method: "cpp-with-size-header",
		Compress: func(t testing.TB, inputPath string) []byte {
			parent := filepath.Dir(inputPath)
			filename := filepath.Base(inputPath)

			cmd := exec.Command("docker", "run", "--rm",
				"-v", fmt.Sprintf("%s:%s", parent, containerDir),
				dockerImage,
				"lzo-tool", "--with-size-header", "-c", filepath.Join(containerDir, filename))

			compressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("lzo-wrapper compression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run lzo-wrapper: %v", err)
			}
			return compressed
		},
		Decompress: func(t testing.TB, inputPath string) []byte {
			parent := filepath.Dir(inputPath)
			filename := filepath.Base(inputPath)

			cmd := exec.Command("docker", "run", "--rm",
				"-v", fmt.Sprintf("%s:%s", parent, containerDir),
				dockerImage,
				"lzo-tool", "--with-size-header", "-d", filepath.Join(containerDir, filename))

			decompressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("lzo-wrapper decompression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run lzo-wrapper decompression: %v", err)
			}
			return decompressed
		},
	}
}

func pythonLzoValidator() validator {
	// this appears to add a variable length header with information, so isn't easy to use
	return validator{
		Method: "python",
		Compress: func(t testing.TB, inputPath string) []byte {
			parent := filepath.Dir(inputPath)
			filename := filepath.Base(inputPath)

			cmd := exec.Command("docker", "run", "--rm",
				"-v", fmt.Sprintf("%s:%s", parent, containerDir),
				dockerImage,
				"lzo-tool.py", "-c", filepath.Join(containerDir, filename))

			compressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("python-lzo compression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run python-lzo: %v", err)
			}
			return compressed
		},
		Decompress: func(t testing.TB, inputPath string) []byte {
			parent := filepath.Dir(inputPath)
			filename := filepath.Base(inputPath)

			cmd := exec.Command("docker", "run", "--rm",
				"-v", fmt.Sprintf("%s:%s", parent, containerDir),
				dockerImage,
				"lzo-tool.py", "-d", filepath.Join(containerDir, filename))

			decompressed, err := cmd.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("python-lzo decompression failed: %v\nStderr: %s", err, exitErr.Stderr)
				}
				t.Fatalf("failed to run python-lzo decompression: %v", err)
			}
			return decompressed
		},
	}
}

func compressFixture(t *testing.T, name string, data []byte) []validator {
	t.Helper()

	absCacheDir, err := filepath.Abs(cacheDir)
	if err != nil {
		t.Fatalf("failed to get absolute cache directory: %v", err)
	}

	inputFile := filepath.Join(absCacheDir, name+".txt")
	if err := os.WriteFile(inputFile, data, 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	var vs []validator

	for _, v := range validators(t) {
		outputFile := v.Path(name)
		if _, err := os.Stat(outputFile); err == nil {
			compressed, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("failed to read cached compressed file: %v", err)
			}
			v.Compressed = compressed
			vs = append(vs, v)
			continue
		}

		// compress the file using the validator's method
		compressed := v.Compress(t, inputFile)
		if err := os.WriteFile(outputFile, compressed, 0644); err != nil {
			t.Fatalf("failed to write compressed file: %v", err)
		}
		t.Logf("compressed file %s using method %s: %d -> %d bytes", outputFile, v.Method, len(data), len(compressed))

		// verify decompression
		absOutputFile, err := filepath.Abs(outputFile)
		if err != nil {
			t.Fatalf("failed to get absolute path for output file: %v", err)
		}
		decompressed := v.Decompress(t, absOutputFile)
		if string(decompressed) != string(data) {
			t.Errorf("decompressed data does not match original data for method %s\nOriginal: %s\nDecompressed: %s",
				v.Method, string(data), string(decompressed))
			if os.Remove(outputFile) != nil {
				t.Logf("failed to remove output file %s", outputFile)
			}
		} else {
			t.Logf("decompression verified successfully for method %s", v.Method)
		}

		v.Compressed = compressed
		vs = append(vs, v)
	}

	return vs
}

func generateTestData(t testing.TB, size int, pattern string) []byte {
	data := make([]byte, size)

	switch pattern {
	case "random":
		if _, err := rand.Read(data); err != nil {
			t.Fatalf("failed to generate random data: %v", err)
		}
	case "repeated":
		for i := range data {
			data[i] = byte(i % 256)
		}
	case "zeros":
		// data is already zero-initialized
	case "text":
		text := []byte("The quick brown fox jumps over the lazy dog. ")
		for i := range data {
			data[i] = text[i%len(text)]
		}
	case "mixed":
		// mix of patterns
		quarter := size / 4
		copy(data[0:quarter], generateTestData(t, quarter, "random"))
		copy(data[quarter:2*quarter], generateTestData(t, quarter, "repeated"))
		copy(data[2*quarter:3*quarter], generateTestData(t, quarter, "zeros"))
		copy(data[3*quarter:], generateTestData(t, size-3*quarter, "text"))
	}

	return data
}

func saveTestCase(t testing.TB, data []byte) {
	// save test to a file for manual inspection in testdata/crash/TIME.bin
	if len(data) == 0 {
		t.Log("No data to save for test case")
		return
	}

	p := filepath.Join("testdata", "crash", fmt.Sprintf("%d.bin", time.Now().Unix()))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatalf("Failed to create directory for test case: %v", err)
	}

	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("Failed to write test case data to file: %v", err)
	}
	t.Logf("Test case saved to %s", p)
}
