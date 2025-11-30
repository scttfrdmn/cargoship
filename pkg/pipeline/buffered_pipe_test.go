package pipeline

import (
	"bytes"
	"io"
	"testing"
)

func TestBufferedPipe_BasicReadWrite(t *testing.T) {
	pr, pw := NewBufferedPipe(1024*1024, 4096) // 1MB buffer, 4KB chunks

	// Write test data
	testData := []byte("Hello, World!")
	go func() {
		n, err := pw.Write(testData)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("Write returned %d, want %d", n, len(testData))
		}
		_ = pw.Close()
	}()

	// Read and verify
	buf := make([]byte, 1024)
	n, err := pr.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read %d bytes, want %d", n, len(testData))
	}
	if !bytes.Equal(buf[:n], testData) {
		t.Errorf("Read data mismatch: got %q, want %q", buf[:n], testData)
	}

	_ = pr.Close()
}

func TestBufferedPipe_LargeData(t *testing.T) {
	// Test with data larger than buffer size
	pr, pw := NewBufferedPipe(64*1024, 8*1024) // 64KB buffer, 8KB chunks

	// Create 1MB of test data
	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Write in goroutine
	go func() {
		n, err := pw.Write(testData)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("Write returned %d, want %d", n, len(testData))
		}
		_ = pw.Close()
	}()

	// Read all data
	var received bytes.Buffer
	buf := make([]byte, 8192)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			received.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	// Verify
	if received.Len() != len(testData) {
		t.Errorf("Received %d bytes, want %d", received.Len(), len(testData))
	}
	if !bytes.Equal(received.Bytes(), testData) {
		t.Errorf("Data mismatch")
	}

	_ = pr.Close()
}

func TestBufferedPipe_CloseWithError(t *testing.T) {
	pr, pw := NewBufferedPipe(1024*1024, 4096)

	// Write some data then close with error
	testError := io.ErrUnexpectedEOF
	go func() {
		_, _ = pw.Write([]byte("test"))
		_ = pw.CloseWithError(testError)
	}()

	// Try to read all data
	buf := make([]byte, 1024)
	for {
		_, err := pr.Read(buf)
		if err != nil {
			// Should eventually get the error
			if err == testError {
				break // Expected error
			}
			if err != io.EOF {
				t.Errorf("Expected error %v, got %v", testError, err)
			}
			break
		}
	}

	_ = pr.Close()
}

func TestBufferedPipe_MultipleWrites(t *testing.T) {
	pr, pw := NewBufferedPipe(1024*1024, 4096)

	// Write multiple times
	chunks := []string{"chunk1", "chunk2", "chunk3", "chunk4"}
	go func() {
		for _, chunk := range chunks {
			_, err := pw.Write([]byte(chunk))
			if err != nil {
				t.Errorf("Write failed: %v", err)
			}
		}
		_ = pw.Close()
	}()

	// Read and verify
	var received bytes.Buffer
	buf := make([]byte, 16)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			received.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	expected := "chunk1chunk2chunk3chunk4"
	if received.String() != expected {
		t.Errorf("Received %q, want %q", received.String(), expected)
	}

	_ = pr.Close()
}

func TestBufferedPipe_ReaderCloseUnblocksWriter(t *testing.T) {
	pr, pw := NewBufferedPipe(1024, 512) // Small buffer to test blocking

	// Write large data that will exceed buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		data := make([]byte, 10*1024) // 10KB, much larger than 1KB buffer
		_, _ = pw.Write(data)
		_ = pw.Close()
	}()

	// Close reader immediately without reading
	_ = pr.Close()

	// Wait for writer to finish - it should unblock
	<-done
}

func BenchmarkBufferedPipe(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pr, pw := NewBufferedPipe(64*1024*1024, 32*1024) // 64MB, 32KB chunks

		go func() {
			_, _ = pw.Write(data)
			_ = pw.Close()
		}()

		buf := make([]byte, 32*1024)
		for {
			_, err := pr.Read(buf)
			if err == io.EOF {
				break
			}
		}
		_ = pr.Close()
	}
}

func BenchmarkBufferedPipe_vs_IOPipe(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB

	b.Run("BufferedPipe", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pr, pw := NewBufferedPipe(64*1024*1024, 32*1024)

			go func() {
				_, _ = pw.Write(data)
				_ = pw.Close()
			}()

			_, _ = io.Copy(io.Discard, pr)
			_ = pr.Close()
		}
	})

	b.Run("io.Pipe", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pr, pw := io.Pipe()

			go func() {
				_, _ = pw.Write(data)
				_ = pw.Close()
			}()

			_, _ = io.Copy(io.Discard, pr)
			_ = pr.Close()
		}
	})
}
