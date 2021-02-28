#!/bin/bash

make -e REGISTRY=fred78290 -e TAG=v1.19.7 container -e GOARCH=amd64
make -e REGISTRY=fred78290 -e TAG=v1.19.7 container -e GOARCH=arm64
