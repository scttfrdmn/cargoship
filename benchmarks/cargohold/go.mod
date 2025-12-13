module github.com/scttfrdmn/cargoship/benchmarks/cargohold

go 1.23

require (
	github.com/scttfrdmn/cargoship v0.6.0
)

// Use local cargoship module during development
replace github.com/scttfrdmn/cargoship => ../..
