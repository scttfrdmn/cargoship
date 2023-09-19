# VHS documentation
# Display:
#   Hide                            Hide the subsequent commands from the output
#   Show                            Show the subsequent commands in the output

Output demo.gif

Require echo


Set Shell "fish"
Set FontSize 16
Set Width 1200
Set Height 700

Hide
Type "rm -rf /tmp/suitcasectl-demo" Enter
Type "clear" Enter
Show

Type "echo 'Thanks for using suitcasectl!'" Sleep 500ms  Enter
Type "mkdir /tmp/suitcasectl-demo" Sleep 500ms Enter
Type "suitcasectl create suitcase --max-suitcase-size=5Mb --destination /tmp/suitcasectl-demo ~/Desktop/example-suitcase" Sleep 500ms Enter

Sleep 15s
