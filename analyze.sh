#!/bin/bash
filename=$1

echo -n "UDP first "
grep UDP "$filename" | grep 'first latency' | awk 'BEGIN {a=0; cnt=0} {a += $7;cnt++} END { print a/cnt }'
echo -n "UDP avg "
grep UDP "$filename" | grep 'total lag' | awk 'BEGIN {a=0; cnt=0} {a += $11;cnt++} END { print a/cnt }'
echo -n "UDP max "
grep UDP "$filename" | grep 'first latency' | awk 'BEGIN {a=0; cnt=0} {a += $11; cnt++} END { print a/cnt }'


echo -n "HTTP first "
grep HTTP "$filename" | grep 'first latency' | awk 'BEGIN {a=0; cnt=0} {a += $7;cnt++} END { print a/cnt }'
echo -n "HTTP avg "
grep HTTP "$filename" | grep 'total lag' | awk 'BEGIN {a=0; cnt=0} {a += $11;cnt++} END { print a/cnt }'
echo -n "HTTP max "
grep HTTP "$filename" | grep 'first latency' | awk 'BEGIN {a=0; cnt=0} {a += $11; cnt++} END { print a/cnt }'

