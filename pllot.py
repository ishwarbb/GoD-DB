import matplotlib.pyplot as plt
import numpy as np

# Latency values
# get_latency = [3412.744, 1788.596, 1198.2, 920.304, 944.352, 1576.548, 1137.136, 957.012, 1204.564, 1077.84]
# put_latency = [4609.996, 2237.712, 1527.912, 1268.624, 1349.72, 2353.504, 1701.168, 1368.62, 1748.0, 1853.876]
get_latency = [3129.396, 993.26, 925.544, 896.984, 556.936, 689.072, 529.392, 713.8, 471.0, 694.388, 786.508, 3263.036, 889.288, 1796.992]
put_latency = [3776.824, 1124.456, 1018.72, 971.42, 830.332, 1032.296, 801.412, 1018.6, 719.556, 968.588, 1098.856, 1126.624, 1232.752, 2728.896]

# Create a range for the x-axis
x = np.arange(len(get_latency))

# Create the plot
plt.figure(figsize=(10, 6))

# Plot get latency
plt.plot(x, get_latency, label='Get Latency', marker='o')

# Plot put latency
plt.plot(x, put_latency, label='Put Latency', marker='o')

# Set labels and title
plt.xlabel('Iteration')
plt.ylabel('Latency (ms)')
plt.title('Get and Put Latency')
plt.legend()

# Show the plot
# Save the plot to a file
plt.savefig('latency_plot.png', bbox_inches='tight')

# Close the plot to free up memory
plt.close()
