<div align="center">
  <h1> World Engine </h1>
</div>

🤷‍♂️

# Starting the Services

To start the chain and nakama runtime, run the following command in the root of the project:

``make start-services``

WARNING: this command will take some time to boot up. Please allow a few minutes for the node and nakama service to fully install all dependencies and start up.

This will boot up a few endpoints for you to interact with. 

### Nakama

The nakama admin dashboard is reachable at `localhost:7351`. From here, you will be able to log-in and access
all the available RPC endpoints for Nakama.

You can use the following default credentials to log-in as admin:

```username: admin```

```password: password```

see https://youtu.be/Ru3RZ6LkJEk for more details


### Cosmos

The default Cosmos endpoints are available to be interacted with:

- gRPC: `localhost:9090`
- REST: `localhost:26657` // currently broken, looking into fixing.. 


## Making Embeddable Contracts

To write contracts and utilize bindings provided by ethermint, we must generate a few specific files. 

NOTE: you must have solc installed.

To generate an ABI, first write your smart contract. Then, run `solc --abi --bin YourContract.sol -o build`.
This will output an `abi` file and a `bin` file in the `build` directory.


# Dependencies

### Proto Generation
- buf: https://docs.buf.build/installation
- proto-doc: `go install github.com/pseudomuto/protoc-gen-doc@latest`
- swagger-gen: `go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger@latest`
- swagger-combine: `npm i swagger-combine -g`
