#!/usr/bin/env python3

import os
import re
import sys
import glob
import matplotlib.pyplot as plt
import numpy as np

def extract_metrics(file_path):
    """Extract metrics from a benchmark result file"""
    metrics = {}
    
    with open(file_path, 'r') as f:
        content = f.read()
        
        # Extract operations per second
        ops_match = re.search(r'Operations/second: ([\d.]+)', content)
        if ops_match:
            metrics['ops_per_sec'] = float(ops_match.group(1))
        
        # Extract total operations
        total_ops_match = re.search(r'Total Operations: (\d+)', content)
        if total_ops_match:
            metrics['total_ops'] = int(total_ops_match.group(1))
        
        # Extract latency
        latency_match = re.search(r'Average Latency: ([\d.]+)(ns|µs|ms|s)', content)
        if latency_match:
            value = float(latency_match.group(1))
            unit = latency_match.group(2)
            
            # Convert to milliseconds for consistency
            if unit == 'ns':
                value /= 1_000_000
            elif unit == 'µs':
                value /= 1_000
            elif unit == 's':
                value *= 1_000
            
            metrics['latency_ms'] = value
        
        # Extract failure rate
        failure_match = re.search(r'Failed Operations: \d+ \(([\d.]+)%\)', content)
        if failure_match:
            metrics['failure_rate'] = float(failure_match.group(1))
        
        # Extract duration
        duration_match = re.search(r'Duration: ([\d.]+)(ns|µs|ms|s|m)s?', content)
        if duration_match:
            value = float(duration_match.group(1))
            unit = duration_match.group(2)
            
            # Convert to seconds
            if unit == 'ns':
                value /= 1_000_000_000
            elif unit == 'µs':
                value /= 1_000_000
            elif unit == 'ms':
                value /= 1_000
            elif unit == 'm':
                value *= 60
            
            metrics['duration_s'] = value
    
    return metrics

def plot_scaling_nodes(results_dir):
    """Plot results for node scaling tests"""
    files = glob.glob(os.path.join(results_dir, 'scaling_*_nodes.txt'))
    if not files:
        print("No node scaling results found")
        return
    
    node_counts = []
    ops_per_sec = []
    latencies = []
    
    for file_path in sorted(files):
        # Extract node count from filename
        match = re.search(r'scaling_(\d+)_nodes', file_path)
        if match:
            node_count = int(match.group(1))
            metrics = extract_metrics(file_path)
            
            node_counts.append(node_count)
            ops_per_sec.append(metrics.get('ops_per_sec', 0))
            latencies.append(metrics.get('latency_ms', 0))
    
    # Create figure with two subplots
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12, 5))
    
    # Plot throughput
    ax1.plot(node_counts, ops_per_sec, 'o-', linewidth=2, markersize=8)
    ax1.set_xlabel('Number of Nodes')
    ax1.set_ylabel('Operations per Second')
    ax1.set_title('Throughput Scaling with Nodes')
    ax1.grid(True)
    
    # Plot latency
    ax2.plot(node_counts, latencies, 'o-', color='red', linewidth=2, markersize=8)
    ax2.set_xlabel('Number of Nodes')
    ax2.set_ylabel('Average Latency (ms)')
    ax2.set_title('Latency with Increasing Nodes')
    ax2.grid(True)
    
    plt.tight_layout()
    plt.savefig(os.path.join(results_dir, 'node_scaling.png'))
    plt.close()
    
    print(f"Node scaling plot saved to {results_dir}/node_scaling.png")

def plot_vnode_scaling(results_dir):
    """Plot results for virtual node scaling tests"""
    files = glob.glob(os.path.join(results_dir, 'scaling_*_vnodes.txt'))
    if not files:
        print("No vnode scaling results found")
        return
    
    vnode_counts = []
    ops_per_sec = []
    
    for file_path in sorted(files):
        # Extract vnode count from filename
        match = re.search(r'scaling_(\d+)_vnodes', file_path)
        if match:
            vnode_count = int(match.group(1))
            metrics = extract_metrics(file_path)
            
            vnode_counts.append(vnode_count)
            ops_per_sec.append(metrics.get('ops_per_sec', 0))
    
    plt.figure(figsize=(10, 6))
    plt.plot(vnode_counts, ops_per_sec, 'o-', linewidth=2, markersize=8)
    plt.xlabel('Virtual Nodes per Physical Node')
    plt.ylabel('Operations per Second')
    plt.title('Performance Impact of Virtual Node Count')
    plt.grid(True)
    
    plt.tight_layout()
    plt.savefig(os.path.join(results_dir, 'vnode_scaling.png'))
    plt.close()
    
    print(f"Virtual node scaling plot saved to {results_dir}/vnode_scaling.png")

def plot_read_write_ratio(results_dir):
    """Plot results for read/write ratio tests"""
    files = glob.glob(os.path.join(results_dir, 'ratio_*_reads.txt'))
    if not files:
        print("No read/write ratio results found")
        return
    
    read_percentages = []
    ops_per_sec = []
    
    for file_path in sorted(files):
        # Extract read percentage from filename
        match = re.search(r'ratio_(\d+)_reads', file_path)
        if match:
            read_pct = int(match.group(1))
            metrics = extract_metrics(file_path)
            
            read_percentages.append(read_pct)
            ops_per_sec.append(metrics.get('ops_per_sec', 0))
    
    plt.figure(figsize=(10, 6))
    plt.plot(read_percentages, ops_per_sec, 'o-', linewidth=2, markersize=8)
    plt.xlabel('Read Percentage (%)')
    plt.ylabel('Operations per Second')
    plt.title('Performance Impact of Read/Write Ratio')
    plt.grid(True)
    plt.xticks(read_percentages)
    
    plt.tight_layout()
    plt.savefig(os.path.join(results_dir, 'read_write_ratio.png'))
    plt.close()
    
    print(f"Read/write ratio plot saved to {results_dir}/read_write_ratio.png")

def plot_quorum_settings(results_dir):
    """Plot results for quorum settings tests"""
    files = glob.glob(os.path.join(results_dir, 'quorum_*.txt'))
    if not files:
        print("No quorum settings results found")
        return
    
    settings = []
    ops_per_sec = []
    latencies = []
    
    for file_path in sorted(files):
        # Extract quorum setting name from filename
        match = re.search(r'quorum_([a-z-]+)\.txt', file_path)
        if match:
            setting = match.group(1)
            metrics = extract_metrics(file_path)
            
            settings.append(setting)
            ops_per_sec.append(metrics.get('ops_per_sec', 0))
            latencies.append(metrics.get('latency_ms', 0))
    
    # Create figure with two subplots
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6))
    
    # Bar positions
    x = np.arange(len(settings))
    width = 0.35
    
    # Plot throughput
    ax1.bar(x, ops_per_sec, width, label='Throughput')
    ax1.set_xlabel('Consistency Setting')
    ax1.set_ylabel('Operations per Second')
    ax1.set_title('Throughput by Consistency Level')
    ax1.set_xticks(x)
    ax1.set_xticklabels(settings)
    ax1.grid(True, axis='y')
    
    # Plot latency
    ax2.bar(x, latencies, width, color='red', label='Latency')
    ax2.set_xlabel('Consistency Setting')
    ax2.set_ylabel('Average Latency (ms)')
    ax2.set_title('Latency by Consistency Level')
    ax2.set_xticks(x)
    ax2.set_xticklabels(settings)
    ax2.grid(True, axis='y')
    
    plt.tight_layout()
    plt.savefig(os.path.join(results_dir, 'quorum_settings.png'))
    plt.close()
    
    print(f"Quorum settings plot saved to {results_dir}/quorum_settings.png")

def plot_node_failure(results_dir):
    """Plot results for node failure tests"""
    before_file = os.path.join(results_dir, 'failure_before.txt')
    after_file = os.path.join(results_dir, 'failure_after.txt')
    
    if not (os.path.exists(before_file) and os.path.exists(after_file)):
        print("Node failure test results not found")
        return
    
    before_metrics = extract_metrics(before_file)
    after_metrics = extract_metrics(after_file)
    
    # Create bar chart comparing before and after
    labels = ['Before Failure', 'After Failure']
    
    fig, (ax1, ax2, ax3) = plt.subplots(1, 3, figsize=(15, 5))
    
    # Throughput comparison
    throughput = [before_metrics.get('ops_per_sec', 0), after_metrics.get('ops_per_sec', 0)]
    ax1.bar(labels, throughput, color=['green', 'blue'])
    ax1.set_ylabel('Operations per Second')
    ax1.set_title('Throughput Before/After Node Failure')
    ax1.grid(True, axis='y')
    
    # Latency comparison
    latency = [before_metrics.get('latency_ms', 0), after_metrics.get('latency_ms', 0)]
    ax2.bar(labels, latency, color=['green', 'blue'])
    ax2.set_ylabel('Average Latency (ms)')
    ax2.set_title('Latency Before/After Node Failure')
    ax2.grid(True, axis='y')
    
    # Failure rate comparison
    failure_rate = [before_metrics.get('failure_rate', 0), after_metrics.get('failure_rate', 0)]
    ax3.bar(labels, failure_rate, color=['green', 'blue'])
    ax3.set_ylabel('Failure Rate (%)')
    ax3.set_title('Failure Rate Before/After Node Failure')
    ax3.grid(True, axis='y')
    
    plt.tight_layout()
    plt.savefig(os.path.join(results_dir, 'node_failure.png'))
    plt.close()
    
    print(f"Node failure comparison plot saved to {results_dir}/node_failure.png")

def main():
    results_dir = "scalability_results"
    if len(sys.argv) > 1:
        results_dir = sys.argv[1]
    
    if not os.path.exists(results_dir):
        print(f"Results directory {results_dir} does not exist")
        sys.exit(1)
    
    print(f"Visualizing results from {results_dir}")
    
    # Generate all plots
    plot_scaling_nodes(results_dir)
    plot_vnode_scaling(results_dir)
    plot_read_write_ratio(results_dir)
    plot_quorum_settings(results_dir)
    plot_node_failure(results_dir)
    
    print("Visualization complete!")

if __name__ == "__main__":
    main() 