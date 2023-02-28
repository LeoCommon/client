#!/usr/bin/bash

forkmagic() {
   while true; do date; command; sleep 1; done
}

trap 'echo "Hey! Leave me alone!"' INT
trap 'echo "No, i dont want to be terminated!"' TERM

# Spawn 4 processes
forkmagic &
forkmagic &
forkmagic &
forkmagic &

# One blocking process so we dont instantly terminate
forkmagic