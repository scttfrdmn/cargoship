# GPG Encryption

Suitcases can be optionally encrypted, if given a `gpg` extension (Example:
`--suitcase-format="tar.gz.gpg"`). By default, they will be encrypted to the
public keys of the Duke University Linux team. This can be disabled with the
`--exclude-systems-pubkeys` flag. To use your own keys, use the `--public-key`
flag. This flag can be used multiple times.
