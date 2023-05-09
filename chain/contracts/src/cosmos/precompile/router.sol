pragma solidity ^0.8.4;

interface IRouter {
    function Send(bytes calldata message, string calldata namespace) external returns (Response memory);

    struct Response {
        uint    Code;
        string  Message;
    }
}
