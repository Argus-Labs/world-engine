#!/usr/bin/env bash

cd proto || exit
buf generate --template buf.gen.doc.yaml
cd ..
