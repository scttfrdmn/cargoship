# Wizard (BETA)

The `suitcasectl wizard` command is meant to simplify a base subset of all the different arguments that can be passed when creating a new suitcase. While not as full featured as `suitcasectl create suitcase ...`, it can be easier than reading through the manual on what all the other features do, to hopefully make onboarding folks easier.

## Demo

![wizard](./vhs/wizard.gif)

## Information for Administrators

If you would like to pre-populate some of the fields for users, you can do so by using a few environment variables:

`SUITCASECTL_SOURCE`

`SUITCASECTL_DESTINATION`

`SUITCASECTL_MAXSIZE`

`SUITCASECTL_TRAVELAGENT`
