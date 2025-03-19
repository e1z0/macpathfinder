<?php
header('Content-Type: application/json');

// Database credentials
$dbFile = '../network_inventory.db'; // database location
$skip = ['Po','Port-Channel','lag']; // which ports to skip


// Function to format MAC address
function formatMacAddress($mac) {
    // Remove any non-alphanumeric characters (like '-' or ':')
    $mac = preg_replace('/[^A-F0-9]/i', '', $mac);

    // Ensure it's exactly 12 characters long (valid MAC address length)
    if (strlen($mac) !== 12) {
        return false; // Invalid MAC address length
    }

    // Convert to uppercase
    $mac = strtoupper($mac);

    // Add colons to the MAC address
    $formattedMac = implode(':', str_split($mac, 2));

    return $formattedMac;
}

// Create a PDO connection
try {
    $pdo = new PDO("sqlite:$dbFile");
    $pdo->setAttribute(PDO::ATTR_ERRMODE, PDO::ERRMODE_EXCEPTION);
} catch (PDOException $e) {
    echo json_encode(["success" => false, "message" => "Database connection failed: " . $e->getMessage()]);
    exit;
}

// Check if MAC address is provided
if (isset($_GET['mac']) && !empty($_GET['mac'])) {
    $macAddress = $_GET['mac'];
    $macAddress = formatMacAddress($macAddress);

    // Prepare the SQL query
    $stmt = $pdo->prepare("SELECT * FROM network_inventory WHERE mac_address = :mac_address");
    $stmt->bindParam(':mac_address', $macAddress, PDO::PARAM_STR);

    // Execute the query
    if ($stmt->execute()) {
        $res = $stmt->fetchAll(PDO::FETCH_ASSOC);
        $result = [];
        foreach ($res as $item) {
             $item["access_val"] = "Trunk port";
             if ($item["access"] == "1") $item["access_val"] = "Access Port";
             // Check if 'name' in the item contains any of the patterns from $arr
             foreach ($skip as $pattern) {
               if (strpos($item['port_name'], $pattern) !== false) {
                  // Skip the item if it matches the pattern
                   continue 2; // Skip to the next item
               }
            }
            $result[] = $item;
       }

        // If result is found, return it
        if ($result) {
            echo json_encode(["success" => true, "result" => $result]);
        } else {
            echo json_encode(["success" => false, "message" => "MAC address not found."]);
        }
    } else {
        echo json_encode(["success" => false, "message" => "Error executing query."]);
    }
} else {
    echo json_encode(["success" => false, "message" => "MAC address is required."]);
}
?>
