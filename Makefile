all:
	date -u +"UTC %Y-%m-%d-%H:%M"
	echo "go test"
	go test -failfast -timeout 4m
	date -u +"UTC %Y-%m-%d-%H:%M"
	echo "go race"
	go test -race -failfast -timeout 4m
	date -u +"UTC %Y-%m-%d-%H:%M"

each: block channel runstop tiar fakedrop fakesuccess race

block:
	go test -failfast -timeout 2m -run TestPinger

channel:
	go test -failfast -timeout 2m --run TestPingerWithStatsChannel 

runstop:
	go test -failfast -timeout 2m --run TestRunStopLoop

shutdown:
	go test -failfast -timeout 60s --run TestPingersShutdown

tiar:
	go test -failfast -timeout 5s --run TestTiarCalculator

fakedrop:
	go test -failfast -timeout 2m --run TestPingerFakeDrop

fakesuccess:
	go test -failfast -timeout 2m --run TestPingerFakeSuccess

race:
	go test -race -failfast -timeout 2m

sync:
	rsync -avz --exclude '.git' ~/go/src/gitlab.edgecastcdn.net/dseddon/icmpengine/ /home/das/go/src/git.vzbuilders.com/dseddon/icmpengine/
