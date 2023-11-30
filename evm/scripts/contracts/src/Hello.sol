pragma solidity ^0.8.13;

import {IRouter} from "./router.sol";

contract Hello {
    string public message;

    function SetMessage(string calldata _msg) public {
        message = _msg;
    }

    function GetMessage() public view returns (string memory) {
        return message;
    }
}