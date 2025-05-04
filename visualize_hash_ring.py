#!/usr/bin/env python3

import matplotlib.pyplot as plt
import numpy as np
import math
import random
import hashlib
import argparse

def hash_key(key, max_val=2**32-1):
    """Hash a key using MD5 and return a value in [0, max_val]"""
    hash_object = hashlib.md5(key.encode())
    hash_hex = hash_object.hexdigest()
    hash_int = int(hash_hex, 16)
    return hash_int % max_val

def generate_physical_nodes(n):
    """Generate n physical node names"""
    return [f"worker{i+1}" for i in range(n)]

def generate_virtual_nodes(physical_nodes, vnodes_per_physical=10):
    """Generate virtual nodes for each physical node"""
    virtual_nodes = {}
    for pnode in physical_nodes:
        vnodes = []
        for i in range(vnodes_per_physical):
            vnode_name = f"{pnode}:vn{i}"
            vnode_hash = hash_key(vnode_name)
            vnodes.append((vnode_name, vnode_hash))
        virtual_nodes[pnode] = vnodes
    return virtual_nodes

def generate_keys(n):
    """Generate n random keys and their hash positions"""
    keys = []
    for i in range(n):
        key = f"key{i}"
        key_hash = hash_key(key)
        keys.append((key, key_hash))
    return keys

def find_responsible_node(key_hash, virtual_nodes_hash_sorted):
    """Find the node responsible for a key"""
    for vnode_hash, vnode_name in virtual_nodes_hash_sorted:
        if key_hash <= vnode_hash:
            return vnode_name
    # If we got here, the key is greater than all node hashes,
    # so it wraps around to the first node
    return virtual_nodes_hash_sorted[0][1]

def visualize_hash_ring(physical_nodes, vnode_count, key_count):
    # Generate virtual nodes
    virtual_nodes = generate_virtual_nodes(physical_nodes, vnode_count)
    
    # Flatten virtual nodes for plotting
    flat_vnodes = []
    for pnode, vnodes in virtual_nodes.items():
        for vnode_name, vnode_hash in vnodes:
            flat_vnodes.append((vnode_hash, vnode_name, pnode))
    
    # Sort by hash
    flat_vnodes.sort(key=lambda x: x[0])
    
    # Generate some random keys
    keys = generate_keys(key_count)
    
    # Create sorted list of (hash, vnode_name) tuples for key assignment
    vnode_hash_sorted = [(vnode[0], vnode[1]) for vnode in flat_vnodes]
    
    # Assign keys to nodes
    key_assignments = {}
    for key, key_hash in keys:
        responsible_vnode = find_responsible_node(key_hash, vnode_hash_sorted)
        # Extract the physical node from the vnode name
        responsible_pnode = responsible_vnode.split(':')[0]
        if responsible_pnode not in key_assignments:
            key_assignments[responsible_pnode] = []
        key_assignments[responsible_pnode].append((key, key_hash))
    
    # Plot configuration
    fig = plt.figure(figsize=(14, 9))
    
    # Create circular plot
    ax1 = fig.add_subplot(111, polar=True)
    
    # Define colors for each physical node
    colors = plt.cm.tab10(np.linspace(0, 1, len(physical_nodes)))
    color_map = {node: colors[i] for i, node in enumerate(physical_nodes)}
    
    # Plot virtual nodes on the ring
    max_hash = 2**32 - 1
    theta = np.array([vnode[0]/max_hash * 2 * np.pi for vnode in flat_vnodes])
    
    # Plot each vnode
    for i, (vnode_hash, vnode_name, pnode) in enumerate(flat_vnodes):
        angle = vnode_hash/max_hash * 2 * np.pi
        ax1.scatter(angle, 1, s=100, color=color_map[pnode], alpha=0.7)
    
    # Plot keys
    for key, key_hash in keys:
        angle = key_hash/max_hash * 2 * np.pi
        ax1.scatter(angle, 0.8, s=20, color='black', alpha=0.5)
    
    # Plot connecting lines from keys to their assigned nodes
    for pnode, assigned_keys in key_assignments.items():
        for key, key_hash in assigned_keys:
            key_angle = key_hash/max_hash * 2 * np.pi
            
            # Find closest vnode
            closest_vnode = None
            min_distance = float('inf')
            
            for vnode_hash, vnode_name, vnode_pnode in flat_vnodes:
                if vnode_pnode == pnode:
                    vnode_angle = vnode_hash/max_hash * 2 * np.pi
                    # Calculate circular distance
                    dist = min(abs(key_angle - vnode_angle), 2*np.pi - abs(key_angle - vnode_angle))
                    if dist < min_distance:
                        min_distance = dist
                        closest_vnode = (vnode_hash, vnode_name)
            
            if closest_vnode:
                vnode_angle = closest_vnode[0]/max_hash * 2 * np.pi
                ax1.plot([key_angle, vnode_angle], [0.8, 1], 
                         color=color_map[pnode], alpha=0.2, linestyle='-')
    
    # Create legend
    legend_elements = [plt.Line2D([0], [0], marker='o', color='w', 
                                  markerfacecolor=color_map[node], markersize=10, 
                                  label=f"{node} ({len(virtual_nodes[node])} vnodes)") 
                      for node in physical_nodes]
    
    # Add key legend
    legend_elements.append(plt.Line2D([0], [0], marker='o', color='w', 
                                      markerfacecolor='black', markersize=6, 
                                      label=f"Keys ({key_count})"))
    
    ax1.legend(handles=legend_elements, loc='upper right', bbox_to_anchor=(0.1, 0.1))
    
    # Add title and remove radial labels
    plt.title(f"Consistent Hash Ring Visualization\n{len(physical_nodes)} Nodes, {vnode_count} Virtual Nodes per Node", fontsize=14)
    ax1.set_yticklabels([])
    
    # Add hash position labels around the circle
    positions = [0, 0.25, 0.5, 0.75]
    position_labels = ["0", f"{max_hash//4}", f"{max_hash//2}", f"{3*max_hash//4}"]
    ax1.set_xticks([pos * 2 * np.pi for pos in positions])
    ax1.set_xticklabels(position_labels)
    
    # Add key distribution analysis
    key_dist = [len(key_assignments.get(node, [])) for node in physical_nodes]
    avg_keys = sum(key_dist) / len(key_dist) if key_dist else 0
    std_keys = np.std(key_dist) if key_dist else 0
    
    plt.figtext(0.5, 0.01, 
                f"Key Distribution - Avg: {avg_keys:.1f} keys/node, Std Dev: {std_keys:.1f}", 
                ha="center", fontsize=12, bbox={"facecolor":"white", "alpha":0.5, "pad":5})
    
    # Display distribution info for each node
    for i, node in enumerate(physical_nodes):
        key_count = len(key_assignments.get(node, []))
        percent = (key_count / len(keys)) * 100 if keys else 0
        plt.figtext(0.02, 0.95 - i*0.03,
                    f"{node}: {key_count} keys ({percent:.1f}%)",
                    color=color_map[node],
                    fontsize=10)
    
    plt.tight_layout()
    plt.savefig("hash_ring_visualization.png", dpi=150)
    print(f"Visualization saved to 'hash_ring_visualization.png'")
    return plt

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Visualize a consistent hash ring")
    parser.add_argument("--nodes", type=int, default=3, help="Number of physical nodes")
    parser.add_argument("--vnodes", type=int, default=10, help="Number of virtual nodes per physical node")
    parser.add_argument("--keys", type=int, default=100, help="Number of keys to place on the ring")
    args = parser.parse_args()
    
    physical_nodes = generate_physical_nodes(args.nodes)
    visualize_hash_ring(physical_nodes, args.vnodes, args.keys)
    plt.show() 