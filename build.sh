#!/bin/bash

make -e REGISTRY=fred78290 -e TAG=v1.18.12 container -e GOARCH=amd64
make -e REGISTRY=fred78290 -e TAG=v1.18.12 container -e GOARCH=arm64
