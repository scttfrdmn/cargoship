# Travel Agent

Travel Agent is a remote API that `suitcasectl` will reach out to for reporting
and instructions. For example, it can be used to:

* Show a user the progress in a WebUI.
* Store the inventory in spot that can be later searched.
* Provide credentials and target information for a copy out to the cloud.

The Duke University implementation uses RToolkits as the Travel Agent endpoint,
however you can write your own as long as it supports the defined endoints.

## Usage

* Go to your Travel Agent Provider (RToolkits if at Duke), and go to the Suitcase and generate a new suitcasectl token.
* Copy past your token in to the CLI where you want to do the transfer, using something like: `suitcasectl create suitcase /foo/bar --travel-agent=<PASTE TOKEN>`

This will report all metadata back to your Travel Agent Provider, then instruct
suitcasectl to copy all the suitcase data to an arbitrary cloud location that
the Travel Agent Provider decides is appropriate.

## Demo

![travel-agent](../vhs/travel-agent.gif)

## Hosting your own Travel Agent Provider

### Web UI

The web interface should contain a method for users to copy/paste the initial token from. This token is the remote url and an access password base64 encoded. Below is an example of a valid token that the provider could give the user:

```bash
$ echo eyAidXJsIjogImh0dHBzOi8vZXhhbXBsZS5jb20vYXBpL3YxL3N1aXRjYXNlX3RyYW5zZmVycy81IiwgInBhc3N3b3JkIjogIjYxZjI0Mzk4LTZkZTctNGFiYi05ODZmLTQ3ZjhjYTk5MjdiNSIgfQo= | base64 --decode | jq
{
  "url": "https://example.com/api/v1/suitcase_transfers/5",
  "password": "61f24398-6de7-4abb-986f-47f8ca9927b5"
}
```

### Rest API

`$base` should be something like `https://example.com/api/v1/` in the below examples

#### Status Update Endpoint

Status should be updated using an http call like the one below:

```plain
curl -X 'PATCH' -d '$update_json' -H 'Authorization: Bearer $PASSWORD' -H 'Content-Type: application/json; charset=utf-8' '$base/suitcase_transfers/25'
```

See the StatusUpdate type
[here](https://pkg.go.dev/gitlab.oit.duke.edu/devil-ops/suitcasectl@main/pkg/travelagent#StatusUpdate)
for valid options in the `$update_json` above

#### Credential Endpoint

This endpoint should provide a JSON response of data that gives suitcasectl enough information on where to send the generated data. For example, a valid response could be:

```json
{
  "auth_type": {
    "type": "azureblob",
    "account": "suitcasectltesting",
    "sas_url": "https://suitcasectltesting.blob.core.windows.net/128-test-suitcase?sp=racwdl&st=2023-10-18T12%3A30%3A04Z&se=2023-10-19T12%3A30%3A04Z&skoid=3e595992-23f9-480f-908b-d697d462ee0c&sktid=cb72c54e-4a31-4d9e-b14a-1ea36dfac94c&s
kt=2023-10-18T12%3A30%3A04Z&ske=2023-10-19T12%3A30%3A04Z&sks=b&skv=2021-08-06&spr=https&sv=2021-08-06&sr=c&sig=REDACTED"
  },
  "destination": "/128-test-suitcase",
  "expire_seconds": 86400
}
```

Expiration is in seconds. This is the time that the credential is valid for.
The credential is requested immediately before transferring each item to the
cloud, and should be long enough for the transfer to complete.

The items in the `auth_type` stanza will be converted to an inline Rclone
destination by suitcasectl.

```plain
curl -X 'GET' -H 'Authorization: Bearer $PASSWORD' -H 'Content-Type: application/json; charset=utf-8' '$base/suitcase_transfers/25/credentials?expiry_seconds=86400'
```
