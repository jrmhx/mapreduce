# MapReduce

## Example to Run (Sequential Version)

```bash
# Build the plugin
go build -buildmode=plugin -o build/wc.so ./plugins/wc/

# Build sequential runner
go build -o build/seq ./cmd/sequential/

# Run
cd build
rm -f mr-out*
./seq wc.so ../testdata/pg*.txt

# View sorted output
sort mr-out-0
```
