<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>MAC Address Search</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            background-color: #f4f4f9;
            padding: 20px;
        }
        h1 {
            text-align: center;
            color: #333;
        }
        .search-container {
            display: flex;
            justify-content: center;
            margin-bottom: 20px;
        }
        .search-input {
            width: 300px;
            padding: 10px;
            border: 2px solid #ddd;
            border-radius: 5px;
            font-size: 16px;
        }
        .search-button {
            padding: 10px 20px;
            background-color: #007BFF;
            color: white;
            border: none;
            border-radius: 5px;
            cursor: pointer;
            margin-left: 10px;
        }
        .search-button:hover {
            background-color: #0056b3;
        }
        .results {
            margin-top: 20px;
            display: flex;
            flex-direction: column;
            align-items: center;
        }
        .result-item {
            background-color: #fff;
            border: 1px solid #ddd;
            border-radius: 5px;
            margin: 5px 0;
            padding: 10px;
            font-size: 16px;
            width: 400px;
        }
        .no-results {
            color: #888;
        }
    </style>
</head>
<body>

    <h1>MAC Address Search</h1>

    <div class="search-container">
        <input type="text" id="macSearchInput" class="search-input" placeholder="Enter MAC address to search...">
        <button class="search-button" onclick="searchMacAddress()">Search</button>
    </div>

    <div class="results" id="results">
        <!-- Results will be displayed here -->
    </div>

    <script>
        function searchMacAddress() {
            const macAddress = document.getElementById('macSearchInput').value.trim();
            const resultsDiv = document.getElementById('results');

            if (!macAddress) {
                resultsDiv.innerHTML = '<p class="no-results">Please enter a MAC address.</p>';
                return;
            }

            // Clear previous results
            resultsDiv.innerHTML = 'Loading...';

            // Fetch data from PHP backend
            fetch(`search_mac.php?mac=${macAddress}`)
                .then(response => response.json())
                .then(data => {
                    resultsDiv.innerHTML = ''; // Clear loading message
                    if (data.success && data.result && data.result.length > 0) {
                        data.result.forEach(item => {
                            resultsDiv.innerHTML += `
                                <div class="result-item">
                                    <strong>Hostname:</strong> ${item.switch_name} <br>
                                    <strong>IP:</strong> ${item.switch_ip} <br>
                                    <strong>Brand:</strong> ${item.vendor} <br>
                                    <strong>MAC Address:</strong> ${item.mac_address} <br>
                                    <strong>Port:</strong> ${item.port_name} <strong>Type:</strong> ${item.access_val} <br>
                                    <strong>Created At:</strong> ${item.created_at} <br>
                                    <strong>Updated At:</strong> ${item.updated_at}
                                </div>
                            `;
                        });
                    } else {
                        resultsDiv.innerHTML = '<p class="no-results">No data found for this MAC address.</p>';
                    }
                })
                .catch(error => {
                    resultsDiv.innerHTML = '<p class="no-results">Error fetching data.</p>';
                    console.error('Error:', error);
                });
        }
    </script>

</body>
</html>
