# Overview

## Inventory

Inventory is a yaml file that contains a list of files and hashes.

```bash
❯ suitcasectl create inventory ~/Desktop/example-suitcase/ > /tmp/inventory.yaml
   • walking directory         dir=/Users/drews/Desktop/example-suitcase/
   • Completed
   • Complete                  end=2022-06-30 09:32:46.60041 -0400 EDT m=+0.078226000 start=2022-06-30 09:32:46.524261 -0400 EDT m=+0.002077607 time=76.148393ms
```

## Suitcase

Suitcase is the giant blob of all the data

```bash
❯ suitcasectl create suitcase -i /tmp/inventory.yaml /tmp/out.tar.gz.gpg
   • Pulling in pubkeys        subdir=linux url=https://gitlab.oit.duke.edu/oit-ssi-systems/staff-public-keys.git
   • Filling Suitcase          destination=/tmp/out.tar.gz.gpg encryptInner=false format=tar.gz.gpg
   • Complete                  end=2022-06-30 09:33:32.506736 -0400 EDT m=+0.212799779 start=2022-06-30 09:33:32.295733 -0400 EDT m=+0.001800883 time=210.998896ms
```

Encryption is optional. Encryption and compression will be based on the target file extension.
