# Suitcase

A suitcase is just a standard tar'd and compressed file. Multiple compression
algorhytms are available, but we use `zst`, as it provided the best parallel
performance.

Suitcases are created in parallel to maximize throughput, and can be read using
standard tools like [GNU Tar](https://www.gnu.org/software/tar/).
