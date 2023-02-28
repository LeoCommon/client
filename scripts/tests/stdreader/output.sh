#!/usr/bin/bash

forkmagic() {
   while true; do echo "date: $(date)"; >&2 echo "error"; command; sleep 1; done
}

# Spawn 4 processes
forkmagic &
forkmagic &
forkmagic &
forkmagic &

# One blocking process so we dont instantly terminate
forkmagic