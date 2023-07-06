pragma solidity ^0.8.4;

interface IRouter {
    function Send(bytes calldata message, string calldata messageID, string calldata namespace) external returns (Response memory);

    struct Response {
        // Code is an arbitrary integer that describes the result of execution on an execution shard.
        uint    Code;
        // Message contains the bytes of an abi encoded struct.
        bytes  Message;
    }
}
