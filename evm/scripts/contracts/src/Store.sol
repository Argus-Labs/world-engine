// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;


contract Store {
    uint public Number;



    function setNumber(uint num) public {
        Number = num;
    }

    function GetNumber() public view returns (uint) {
        return Number;
    }
}