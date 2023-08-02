pragma solidity ^0.8.4;

interface IRouter {
    function send(bytes calldata message, uint64 messageID, string calldata namespace) external returns (bytes memory);
}
