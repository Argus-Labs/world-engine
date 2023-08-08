pragma solidity ^0.8.4;

interface IRouter {
    function sendMessage(bytes calldata message, uint64 messageID, string calldata namespace)
        external
        returns (bytes memory);
    function query(bytes calldata request, string calldata resource, string calldata namespace)
        external
        returns (bytes memory);
}
