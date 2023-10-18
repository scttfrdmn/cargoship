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

=== "Homebrew"

    ```plain
    brew tap devil-ops/devil-ops https://gitlab.oit.duke.edu/devil-ops/homebrew-devil-ops.git
    brew install suitcasectl
    ```

=== "Yum"

    ```plain
    echo '[devil-ops]
    name=devil-ops
    baseurl=https://oneget.oit.duke.edu/rpm/devil-ops-rpms/
    gpgkey=https://gitlab.oit.duke.edu/devil-ops/installing-devil-ops-packages/-/raw/main/pubkeys/RPM-GPG-KEY-DEVIL-OPS-2022-05-02
    gpgcheck=1
    enabled=1' | sudo tee /etc/yum.repos.d/devil-ops.repo
    sudo yum install suitcasectl
    ```

=== "Apt"

    ```plain
    wget -qO - https://oneget.oit.duke.edu/debian-feeds/devil-ops-debs.pub | sudo gpg --dearmor -o /etc/apt/keyrings/devil-ops-debs.gpg
    echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/devil-ops-debs.gpg] https://oneget.oit.duke.edu/ devil-ops-debs main" | sudo tee /etc/apt/sources.list.d/devil-ops.list
    sudo apt update
    sudo apt install suitcasectl
    ```

    Ubuntu 22.04 and later

=== "Apt Legacy"

    ```plain
    wget -qO - https://oneget.oit.duke.edu/debian-feeds/devil-ops-debs.pub | sudo apt-key add -
    echo "deb https://oneget.oit.duke.edu/ devil-ops-debs main" | sudo tee /etc/apt/sources.list.d/devil-ops.list
    sudo apt update
    sudo apt install suitcasectl
    ```

    Ubuntu 20.04 and earlier

=== "Local builds"

    ```plain
    go install gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl@main
    ```

    You can also use `go install` to download and build the latest commits to `main` (Or any other branch/tag)
