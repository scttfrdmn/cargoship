# CLI Output

The console output from the `suitcasectl` app will also be logged in json
format to the directory containing the suitcases. This will be in a file called
`suitcasectl.log`

Example:

```json
{"level":"info","override-file":"/Users/drews/Desktop/example-suitcase/suitcasectl.yaml","time":1695130800,"message":"Found user overrides, using them"}
{"level":"info","index":1,"file-count":11,"file-size":3662270,"file-size-human":"3.7 MB","time":1695130800,"message":"🧳 suitcase archive created"}
{"level":"info","index":2,"file-count":3,"file-size":3379621,"file-size-human":"3.4 MB","time":1695130800,"message":"🧳 suitcase archive created"}
{"level":"info","index":3,"file-count":1,"file-size":3145728,"file-size-human":"3.1 MB","time":1695130800,"message":"🧳 suitcase archive created"}
{"level":"info","file-count":15,"file-size":10187619,"file-size-human":"10 MB","time":1695130800,"message":"🧳 total suitcase archives"}
{"level":"warn","time":1695130800,"message":"Only creating inventory file, no suitcase archives"}
```
