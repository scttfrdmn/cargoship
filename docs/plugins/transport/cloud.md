# Copy to the Cloud ☁️

Suitcasectl currently supports cloud destinations that are supported by the
[rclone](https://github.com/rclone/rclone) software library. While you don't
need to have Rclone installed, you will need to have your destination defined
in `~/.config/rclone/rclone.conf`, or using environment variables.

For example, if you have:

```shell
❯ cat ~/.config/rclone/rclone.conf
[suitcasectl-azure]
type = azureblob
account = suitcasectltesting
key = your-key
```

You can copy your files with:

```shell
❯ suitcasectl rclone ~/my-suitcases/ suitcasectl-azure:/test/
...
```

To pull the destination information from environment variables, use the following:

```shell export RCLONE_CONFIG_MYCLOUD_TYPE=azureblob
export RCLONE_CONFIG_MYCLOUD_ACCOUNT=suitcasectltesting
export RCLONE_CONFIG_MYCLOUD_KEY=your-key
❯ suitcasectl rclone ~/my-suitcases/ my-cloud:/test/
...

```

Or you can copy them up as they are created with the `--cloud-destination`
flag:

```shell
❯ suitcasectl create suitcase ~/example-directory/ --cloud-destination suitcasectl-azure:/test/
...
```

To hopefully prevent later errors, the `--cloud-destination` option also checks
to ensure the destination already exists. If it does not, the command will fail
before anything is created. Using this option also uploads all relevant
metadata that is created with the suitcases
