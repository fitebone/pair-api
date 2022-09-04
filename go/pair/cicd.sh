#!/bin/bash

#protoc --go_out=. ./proto/pairapi.proto 
#protoc --go-grpc_out=. ./proto/pairapi.proto 

echo [CI] BEGIN
docker build -t fitebone/pair-api:dev .
echo [CI] COMPLETE

if [ "$1" = "run" ]
then
    echo Nah LOL idiot
    #docker run -p 50051:50051 fitebone/pair-api:dev
elif [ "$1" = "d" ]
then
    echo [CD] BEGIN
    docker save -o ./deploy/pair-api_dev.tar fitebone/pair-api:dev
    # should use -d flag for detached mode
    cat ./deploy/pair_dev.tar | ssh -C -p [port] -i ./deploy/pairssh [person@ipv4] docker load
    #docker import - custom_docker/image
    echo [CD] COMPLETE
fi