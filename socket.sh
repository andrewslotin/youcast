#!/bin/bash

ssh -nNT -L $(pwd)/docker.sock:/var/run/docker.sock nas.local && rm $(pwd)/docker.sock
