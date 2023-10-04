# VHS documentation
# Display:
#   Hide                            Hide the subsequent commands from the output
#   Show                            Show the subsequent commands in the output

Output travel-agent.gif

Require echo


Set Shell "fish"
Set FontSize 16
Set Width 1200
Set Height 700

Hide
Type "rm -rf /tmp/suitcasectl-demo" Enter
Type "clear" Enter
Show

Type "echo 'Get your Travel Agent Token from RToolkits Web Interface!'" Sleep 500ms  Enter
Type "echo 'It is just a base64 encoded url and credential pair'" Sleep 500ms Enter
Set TypingSpeed 1ms
# Note this is no longer a good credential, will need to be replaced if regenerating this gif
Type "echo ewogICJ1cmwiOiAiaHR0cHM6Ly9ydG9vbGtpdHMtdGVzdC53ZWIuZHVrZS5lZHUvYXBpL3YxL3N1aXRjYXNlX3RyYW5zZmVycy8xNyIsCiAgInBhc3N3b3JkIjogIjdlM2JiMmQxLWQ4OTgtNGY0ZS04Zjg4LWUxNTAzOWY3YWFmMSIKfQ== | base64 --decode" Sleep 500ms Enter
Sleep 5s
Set TypingSpeed 5ms
Type "suitcasectl create suitcase --max-suitcase-size=5Mb --cloud-destination=/tmp/suitcasectl-demo ~/Desktop/example-suitcase --travel-agent=ewogICJ1cmwiOiAiaHR0cHM6Ly9ydG9vbGtpdHMtdGVzdC53ZWIuZHVrZS5lZHUvYXBpL3YxL3N1aXRjYXNlX3RyYW5zZmVycy8xNyIsCiAgInBhc3N3b3JkIjogIjdlM2JiMmQxLWQ4OTgtNGY0ZS04Zjg4LWUxNTAzOWY3YWFmMSIKfQ== --travel-agent-skip-finalize" Sleep 500ms Enter

Sleep 15s

