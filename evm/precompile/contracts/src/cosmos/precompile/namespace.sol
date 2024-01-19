pragma solidity ^0.8.4;

interface INamespace {
    function register(string memory namespace, string memory gRPCAddress) external returns (bool);

    function addressForNamespace(string memory namespace) external view returns (string memory);
}
