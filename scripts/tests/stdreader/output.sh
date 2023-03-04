#!/usr/bin/bash

forkmagic() {
   while true; do sleep 0.05; echo "date: $(date)"; >&2 echo "error"; command; done
}

# Spawn 4 processes
forkmagic &
forkmagic &
forkmagic &
forkmagic &

# One blocking process so we dont instantly terminate
forkmagic