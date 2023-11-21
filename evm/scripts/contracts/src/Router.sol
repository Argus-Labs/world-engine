// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

interface IRouter {
    function sendMessage(bytes calldata message, uint64 messageID, string calldata namespace) external returns (bool);

    function messageResult(string calldata txHash) external returns (bytes memory, string memory, uint32);

    function query(bytes calldata request, string calldata resource, string calldata namespace)
    external
    returns (bytes memory);
}
