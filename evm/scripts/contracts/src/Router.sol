pragma solidity ^0.8.4;

interface IRouter {
    function sendMessage(bytes memory message, string memory messageID, string memory namespace) external returns (bool);

    function messageResult(string memory txHash) external returns (bytes memory, string memory, uint32);

    function query(bytes memory request, string memory resource, string memory namespace)
    external
    returns (bytes memory);
}
