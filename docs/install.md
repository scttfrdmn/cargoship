# Installation

## Prebuilt Binaries

Prebuilt binaries are the preferred and easiest way to get suitcasectl on your
host. If there is no available prebuilt option for your OS, please [create a new
issue](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/issues/new) and we'll
get it in there!

Download a binary from the
[releases](https://gitlab.oit.duke.edu/devil-ops/suitcasectl/-/releases) page.
This app is a single binary, no other packages or libraries required, so just
make sure it lands somewhere in your `$PATH`.

Want to stay up to date with the latest release without having to check back
here all the time? Use the [devil-ops
package](https://gitlab.oit.duke.edu/devil-ops/installing-devil-ops-packages)
for homebrew, yum, apt, etc..

## Local builds

You can also use `go install` to download and build the latest commits to `main` (Or any other branch/tag)

```plain
go install gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl@main
```
