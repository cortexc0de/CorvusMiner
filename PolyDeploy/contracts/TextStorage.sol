// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @title TextStorage
/// @notice Stores a single text string on Polygon. Only the deploying address
///         may update it; anyone may read it for free.
contract TextStorage {
    string  private _text;
    address public  owner;

    event TextUpdated(address indexed updatedBy, string newText);

    /// @param initialText The text to store at deployment time.
    constructor(string memory initialText) {
        _text = initialText;
        owner = msg.sender;
    }

    /// @notice Replace the stored text. Reverts if called by anyone other than owner.
    /// @param newText The replacement string.
    function setText(string calldata newText) external {
        require(msg.sender == owner, "TextStorage: caller is not owner");
        _text = newText;
        emit TextUpdated(msg.sender, newText);
    }

    /// @notice Return the currently stored text string.
    function getText() external view returns (string memory) {
        return _text;
    }
}
