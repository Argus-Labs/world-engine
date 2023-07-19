pragma solidity ^0.8.4;

interface IRouter {
    function Send(bytes calldata message, uint64 messageID, string calldata namespace) external;
}
