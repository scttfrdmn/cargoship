package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// generateReport creates HTML and text comparison reports
func generateReport(resultsDir, scenario string, results []BenchmarkResult) error {
	// Generate text summary
	if err := generateTextReport(resultsDir, scenario, results); err != nil {
		return err
	}

	// Generate HTML report
	if err := generateHTMLReport(resultsDir, scenario, results); err != nil {
		return err
	}

	return nil
}

// generateTextReport creates a text-based comparison report
func generateTextReport(resultsDir, scenario string, results []BenchmarkResult) error {
	filename := filepath.Join(resultsDir, fmt.Sprintf("%s_report.txt", scenario))
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "CargoHold Performance Benchmark Report\n")
	fmt.Fprintf(f, "=======================================\n\n")
	fmt.Fprintf(f, "Scenario: %s\n", scenario)
	fmt.Fprintf(f, "Generated: %s\n\n", time.Now().Format(time.RFC3339))

	// Sort results by upload time (fastest first)
	sorted := make([]BenchmarkResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UploadDuration < sorted[j].UploadDuration
	})

	fmt.Fprintf(f, "Upload Performance Rankings\n")
	fmt.Fprintf(f, "----------------------------\n")
	for i, result := range sorted {
		toolName := result.Tool
		if result.Strategy != "" {
			toolName = fmt.Sprintf("%s (%s)", result.Tool, result.Strategy)
		}

		fmt.Fprintf(f, "%d. %s\n", i+1, toolName)
		fmt.Fprintf(f, "   Time: %s\n", result.UploadDuration.Round(time.Millisecond))
		fmt.Fprintf(f, "   Throughput: %.1f MB/s\n", result.UploadThroughputMBps)
		fmt.Fprintf(f, "   Memory: %.1f MB\n", result.PeakMemoryMB)
		fmt.Fprintf(f, "   CPU: %.1f%%\n", result.AvgCPUPercent)

		if i == 0 {
			fmt.Fprintf(f, "   ⭐ FASTEST\n")
		} else {
			slowdownPct := (result.UploadDuration.Seconds()/sorted[0].UploadDuration.Seconds() - 1.0) * 100
			fmt.Fprintf(f, "   %.1f%% slower than fastest\n", slowdownPct)
		}
		fmt.Fprintf(f, "\n")
	}

	// Find best CargoHold strategy
	var bestCargoHold *BenchmarkResult
	for _, result := range sorted {
		if result.Tool == "cargohold" {
			bestCargoHold = &result
			break
		}
	}

	if bestCargoHold != nil {
		fmt.Fprintf(f, "\nCargoHold Best Strategy: %s\n", bestCargoHold.Strategy)
		fmt.Fprintf(f, "----------------------------\n")
		fmt.Fprintf(f, "Time: %s\n", bestCargoHold.UploadDuration.Round(time.Millisecond))
		fmt.Fprintf(f, "Throughput: %.1f MB/s\n", bestCargoHold.UploadThroughputMBps)

		// Compare to each competitor
		fmt.Fprintf(f, "\nComparison vs Competitors:\n")
		for _, result := range sorted {
			if result.Tool == "cargohold" {
				continue
			}

			speedup := result.UploadDuration.Seconds() / bestCargoHold.UploadDuration.Seconds()
			fmt.Fprintf(f, "  vs %s: %.2fx faster\n", result.Tool, speedup)
		}
	}

	// Throughput comparison
	fmt.Fprintf(f, "\n\nThroughput Comparison\n")
	fmt.Fprintf(f, "---------------------\n")
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UploadThroughputMBps > sorted[j].UploadThroughputMBps
	})

	for _, result := range sorted {
		toolName := result.Tool
		if result.Strategy != "" {
			toolName = fmt.Sprintf("%s (%s)", result.Tool, result.Strategy)
		}
		fmt.Fprintf(f, "%-30s: %8.1f MB/s\n", toolName, result.UploadThroughputMBps)
	}

	// Resource usage comparison
	fmt.Fprintf(f, "\n\nResource Usage\n")
	fmt.Fprintf(f, "--------------\n")
	fmt.Fprintf(f, "%-30s %12s %12s\n", "Tool", "Peak Memory", "Avg CPU")
	for _, result := range results {
		toolName := result.Tool
		if result.Strategy != "" {
			toolName = fmt.Sprintf("%s (%s)", result.Tool, result.Strategy)
		}
		fmt.Fprintf(f, "%-30s %10.1f MB %10.1f%%\n",
			toolName, result.PeakMemoryMB, result.AvgCPUPercent)
	}

	return nil
}

// generateHTMLReport creates an HTML report with charts
func generateHTMLReport(resultsDir, scenario string, results []BenchmarkResult) error {
	filename := filepath.Join(resultsDir, fmt.Sprintf("%s_report.html", scenario))

	tmpl := template.Must(template.New("report").Parse(htmlTemplate))

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	data := struct {
		Scenario  string
		Generated string
		Results   []BenchmarkResult
		ChartData string
	}{
		Scenario:  scenario,
		Generated: time.Now().Format("2006-01-02 15:04:05"),
		Results:   results,
		ChartData: generateChartData(results),
	}

	return tmpl.Execute(f, data)
}

func generateChartData(results []BenchmarkResult) string {
	// Generate JSON data for Chart.js
	labels := []string{}
	uploadTimes := []float64{}
	throughputs := []float64{}
	memoryUsage := []float64{}

	for _, result := range results {
		label := result.Tool
		if result.Strategy != "" {
			label = fmt.Sprintf("%s (%s)", result.Tool, result.Strategy)
		}
		labels = append(labels, label)
		uploadTimes = append(uploadTimes, result.UploadDuration.Seconds())
		throughputs = append(throughputs, result.UploadThroughputMBps)
		memoryUsage = append(memoryUsage, result.PeakMemoryMB)
	}

	data := map[string]interface{}{
		"labels":      labels,
		"uploadTimes": uploadTimes,
		"throughputs": throughputs,
		"memory":      memoryUsage,
	}

	jsonData, _ := json.Marshal(data)
	return string(jsonData)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>CargoHold Benchmark Report - {{.Scenario}}</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@3.9.1/dist/chart.min.js"></script>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        h1 {
            color: #333;
            border-bottom: 3px solid #4CAF50;
            padding-bottom: 10px;
        }
        .meta {
            color: #666;
            margin-bottom: 30px;
        }
        .chart-container {
            background: white;
            padding: 20px;
            margin: 20px 0;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        table {
            width: 100%;
            background: white;
            border-collapse: collapse;
            margin: 20px 0;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background: #4CAF50;
            color: white;
        }
        tr:hover {
            background: #f5f5f5;
        }
        .winner {
            background: #e8f5e9 !important;
            font-weight: bold;
        }
        .summary {
            background: white;
            padding: 20px;
            margin: 20px 0;
            border-radius: 8px;
            border-left: 4px solid #4CAF50;
        }
    </style>
</head>
<body>
    <h1>🚀 CargoHold Performance Benchmark Report</h1>
    <div class="meta">
        <strong>Scenario:</strong> {{.Scenario}}<br>
        <strong>Generated:</strong> {{.Generated}}
    </div>

    <div class="summary">
        <h2>Summary</h2>
        <p>Comparison of CargoHold against competitors for the {{.Scenario}} scenario.</p>
    </div>

    <h2>📊 Upload Time Comparison</h2>
    <div class="chart-container">
        <canvas id="uploadTimeChart"></canvas>
    </div>

    <h2>⚡ Throughput Comparison</h2>
    <div class="chart-container">
        <canvas id="throughputChart"></canvas>
    </div>

    <h2>💾 Memory Usage Comparison</h2>
    <div class="chart-container">
        <canvas id="memoryChart"></canvas>
    </div>

    <h2>📋 Detailed Results</h2>
    <table>
        <thead>
            <tr>
                <th>Tool</th>
                <th>Strategy</th>
                <th>Upload Time</th>
                <th>Throughput (MB/s)</th>
                <th>Peak Memory (MB)</th>
                <th>Avg CPU (%)</th>
                <th>Errors</th>
            </tr>
        </thead>
        <tbody>
            {{range .Results}}
            <tr>
                <td>{{.Tool}}</td>
                <td>{{.Strategy}}</td>
                <td>{{.UploadDuration}}</td>
                <td>{{printf "%.1f" .UploadThroughputMBps}}</td>
                <td>{{printf "%.1f" .PeakMemoryMB}}</td>
                <td>{{printf "%.1f" .AvgCPUPercent}}</td>
                <td>{{.ErrorCount}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>

    <script>
        const chartData = {{.ChartData}};

        // Upload Time Chart
        new Chart(document.getElementById('uploadTimeChart'), {
            type: 'bar',
            data: {
                labels: chartData.labels,
                datasets: [{
                    label: 'Upload Time (seconds)',
                    data: chartData.uploadTimes,
                    backgroundColor: 'rgba(76, 175, 80, 0.6)',
                    borderColor: 'rgba(76, 175, 80, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: {
                        display: true,
                        text: 'Upload Time (Lower is Better)'
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });

        // Throughput Chart
        new Chart(document.getElementById('throughputChart'), {
            type: 'bar',
            data: {
                labels: chartData.labels,
                datasets: [{
                    label: 'Throughput (MB/s)',
                    data: chartData.throughputs,
                    backgroundColor: 'rgba(33, 150, 243, 0.6)',
                    borderColor: 'rgba(33, 150, 243, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: {
                        display: true,
                        text: 'Throughput (Higher is Better)'
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });

        // Memory Chart
        new Chart(document.getElementById('memoryChart'), {
            type: 'bar',
            data: {
                labels: chartData.labels,
                datasets: [{
                    label: 'Peak Memory (MB)',
                    data: chartData.memory,
                    backgroundColor: 'rgba(255, 152, 0, 0.6)',
                    borderColor: 'rgba(255, 152, 0, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: {
                        display: true,
                        text: 'Memory Usage (Lower is Better)'
                    }
                },
                scales: {
                    y: {
                        beginAtZero: true
                    }
                }
            }
        });
    </script>
</body>
</html>
`
