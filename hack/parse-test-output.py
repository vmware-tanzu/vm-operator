#!/usr/bin/env python3

"""
© Broadcom. All Rights Reserved.
The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
SPDX-License-Identifier: Apache-2.0
"""

"""
Test Log Analyzer for VM Operator

Analyzes GitHub Actions test logs to generate comprehensive timing reports.
Extracts test suite timings, individual test case durations, and generates
detailed markdown reports with multiple views and statistics.
"""

import argparse
import json
import re
import sys
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Tuple, Any


class TestLogAnalyzer:
    """Analyzes test logs and generates timing reports."""

    def __init__(self, log_file: str, verbose: bool = False):
        self.log_file = log_file
        self.verbose = verbose
        self.suites = []
        self.test_cases = []
        self.current_suite = None

        # Regex patterns for parsing
        self.patterns = {
            'suite': re.compile(r'Running Suite: (.+?) - (.+)'),
            'before_suite': re.compile(r'\[BeforeSuite\] PASSED \[([0-9.]+) seconds\]'),
            'after_suite': re.compile(r'\[AfterSuite\] PASSED \[([0-9.]+) seconds\]'),
            'suite_summary': re.compile(r'Ran (\d+) of (\d+) Specs in ([0-9.]+) seconds'),
            'test_case': re.compile(r'^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z).*• \[([0-9.]+) seconds\]'),
            'test_name': re.compile(r'^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s+(.+?)(?:\s+\[0m|\s*$)'),
        }

    def parse_logs(self) -> Dict[str, Any]:
        """Parse test logs and extract timing information."""
        if self.verbose:
            print(f"Parsing log file: {self.log_file}")

        with open(self.log_file, 'r') as f:
            lines = f.readlines()

        for i, line in enumerate(lines):
            self._process_line(line, i, lines)

        # Add last suite if exists
        if self.current_suite:
            self.suites.append(self.current_suite)

        # Calculate summary statistics
        summary = self._calculate_summary()

        if self.verbose:
            print(f"Found {len(self.suites)} test suites")
            print(f"Found {len(self.test_cases)} test cases")
            print(f"Total test execution time: {summary['total_test_time']:.2f} seconds")

        return {
            'suites': self.suites,
            'test_cases': self.test_cases,
            'summary': summary
        }

    def _process_line(self, line: str, index: int, lines: List[str]):
        """Process a single log line."""
        # Check for suite start
        suite_match = self.patterns['suite'].search(line)
        if suite_match:
            if self.current_suite:
                self.suites.append(self.current_suite)
            self.current_suite = {
                'name': suite_match.group(1),
                'path': suite_match.group(2),
                'start_line': index,
                'tests': [],
                'before_suite': 0,
                'after_suite': 0,
                'total_time': 0,
                'num_specs': 0
            }
            return

        # Check for BeforeSuite timing
        before_match = self.patterns['before_suite'].search(line)
        if before_match and self.current_suite:
            self.current_suite['before_suite'] = float(before_match.group(1))
            return

        # Check for AfterSuite timing
        after_match = self.patterns['after_suite'].search(line)
        if after_match and self.current_suite:
            self.current_suite['after_suite'] = float(after_match.group(1))
            return

        # Check for suite summary
        summary_match = self.patterns['suite_summary'].search(line)
        if summary_match and self.current_suite:
            self.current_suite['num_specs'] = int(summary_match.group(1))
            self.current_suite['total_time'] = float(summary_match.group(3))
            return

        # Check for test case timing
        test_match = self.patterns['test_case'].search(line)
        if test_match:
            timestamp = test_match.group(1)
            duration = float(test_match.group(2))

            # Try to find test name in previous lines
            test_name = self._extract_test_name(lines, index)

            test_case = {
                'name': test_name,
                'duration': duration,
                'timestamp': timestamp,
                'suite': self.current_suite['name'] if self.current_suite else 'Unknown Suite'
            }
            self.test_cases.append(test_case)
            if self.current_suite:
                self.current_suite['tests'].append(test_case)

    def _extract_test_name(self, lines: List[str], current_index: int) -> str:
        """Extract test name from surrounding lines."""
        test_name = "Unknown Test"
        for j in range(max(0, current_index - 5), current_index):
            name_match = self.patterns['test_name'].search(lines[j])
            if name_match and not re.search(r'• \[', lines[j]):
                potential_name = name_match.group(2)
                # Clean up ANSI color codes
                potential_name = re.sub(r'\[38;5;\d+m', '', potential_name)
                potential_name = re.sub(r'\[0m', '', potential_name)
                potential_name = re.sub(r'\[1m', '', potential_name)
                potential_name = potential_name.strip()
                if potential_name and not potential_name.startswith('I0') and not potential_name.startswith('E0'):
                    test_name = potential_name
                    break
        return test_name

    def _calculate_summary(self) -> Dict[str, float]:
        """Calculate summary statistics."""
        total_test_time = sum(tc['duration'] for tc in self.test_cases)
        total_before_suite = sum(s['before_suite'] for s in self.suites)
        total_after_suite = sum(s['after_suite'] for s in self.suites)
        total_suite_time = sum(s['total_time'] for s in self.suites)

        return {
            'total_test_time': total_test_time,
            'total_before_suite': total_before_suite,
            'total_after_suite': total_after_suite,
            'total_suite_time': total_suite_time
        }


class ReportGenerator:
    """Generates markdown reports from parsed test data."""

    def __init__(self, data: Dict[str, Any], github_url: str = None, run_time: str = None, include_mermaid: bool = True):
        self.data = data
        self.suites = data['suites']
        self.test_cases = data['test_cases']
        self.summary = data['summary']
        self.github_url = github_url
        self.run_time = run_time
        self.include_mermaid = include_mermaid

    def generate_markdown(self, output_file: str, include_instructions: bool = True):
        """Generate comprehensive markdown report."""
        report = []

        if include_instructions:
            report.extend(self._generate_reproduction_instructions())

        report.extend(self._generate_header())
        report.extend(self._generate_overall_summary())

        if self.include_mermaid:
            report.extend(self._generate_mermaid_overview())
            report.extend(self._generate_duration_distribution_chart())

        report.extend(self._generate_suite_timing_table())

        if self.include_mermaid:
            report.extend(self._generate_suite_timing_chart())

        report.extend(self._generate_setup_teardown_tables())
        report.extend(self._generate_slowest_tests())

        if self.include_mermaid:
            report.extend(self._generate_test_duration_histogram())

        report.extend(self._generate_duration_distribution())
        report.extend(self._generate_suite_breakdown())

        if self.include_mermaid:
            report.extend(self._generate_execution_timeline())

        report.extend(self._generate_fastest_tests())
        report.extend(self._generate_performance_insights())

        if self.include_mermaid:
            report.extend(self._generate_time_breakdown_pie())

        # Write the report
        with open(output_file, 'w') as f:
            f.write('\n'.join(report))

        return len(report)

    def _generate_mermaid_overview(self) -> List[str]:
        """Generate Mermaid overview chart."""
        lines = []
        lines.append("## Test Execution Overview")
        lines.append("")
        lines.append("```mermaid")
        lines.append("graph LR")
        lines.append(f"    A[Total Execution<br/>{self.summary['total_suite_time']:.0f}s] --> B[Test Cases<br/>{self.summary['total_test_time']:.0f}s]")
        lines.append(f"    A --> C[Setup Time<br/>{self.summary['total_before_suite']:.0f}s]")
        lines.append(f"    A --> D[Teardown Time<br/>{self.summary['total_after_suite']:.0f}s]")
        lines.append(f"    B --> E[{len(self.test_cases)} Tests]")
        lines.append(f"    C --> F[{len([s for s in self.suites if s['before_suite'] > 0])} BeforeSuite]")
        lines.append(f"    D --> G[{len([s for s in self.suites if s['after_suite'] > 0])} AfterSuite]")
        lines.append("```")
        lines.append("")
        return lines

    def _generate_duration_distribution_chart(self) -> List[str]:
        """Generate XY chart for test duration distribution."""
        lines = []
        lines.append("## Test Duration Distribution Chart")
        lines.append("")
        lines.append("```mermaid")
        lines.append("---")
        lines.append("config:")
        lines.append("  themeVariables:")
        lines.append("    xyChart:")
        lines.append("      backgroundColor: \"transparent\"")
        lines.append("---")
        lines.append("xychart-beta")
        lines.append('    title "Test Duration Distribution"')
        lines.append('    x-axis ["0-1s", "1-5s", "5-10s", "10-20s", "20-30s", "30-60s", ">60s"]')

        ranges = [
            (0, 1), (1, 5), (5, 10), (10, 20), (20, 30), (30, 60), (60, float('inf'))
        ]
        counts = []
        for low, high in ranges:
            count = len([t for t in self.test_cases if low <= t['duration'] < high])
            counts.append(count)

        lines.append(f'    y-axis "Number of Tests" 0 --> {max(counts) + 100}')
        lines.append(f'    bar [{", ".join(str(c) for c in counts)}]')
        lines.append("```")
        lines.append("")
        return lines

    def _generate_suite_timing_chart(self) -> List[str]:
        """Generate XY chart for suite execution times."""
        lines = []
        lines.append("## Top 10 Slowest Test Suites")
        lines.append("")
        lines.append("```mermaid")
        lines.append("---")
        lines.append("config:")
        lines.append("  themeVariables:")
        lines.append("    xyChart:")
        lines.append("      backgroundColor: \"transparent\"")
        lines.append("---")
        lines.append("xychart-beta horizontal")
        lines.append('    title "Top 10 Slowest Test Suites (seconds)"')

        sorted_suites = sorted([s for s in self.suites if s['total_time'] > 0],
                              key=lambda x: x['total_time'], reverse=True)[:10]

        suite_names = []
        suite_times = []
        for suite in sorted_suites:
            name = suite['name'][:20] + '...' if len(suite['name']) > 20 else suite['name']
            suite_names.append(f'"{name}"')
            suite_times.append(f"{suite['total_time']:.1f}")

        lines.append(f'    x-axis [{", ".join(suite_names)}]')
        lines.append(f'    y-axis "Time (seconds)" 0 --> {float(suite_times[0]) + 50}')
        lines.append(f'    bar [{", ".join(suite_times)}]')
        lines.append("```")
        lines.append("")
        return lines

    def _generate_test_duration_histogram(self) -> List[str]:
        """Generate histogram for individual test durations."""
        lines = []
        lines.append("## Test Duration Histogram")
        lines.append("")

        # Create percentile chart
        lines.append("```mermaid")
        lines.append("---")
        lines.append("config:")
        lines.append("  themeVariables:")
        lines.append("    xyChart:")
        lines.append("      backgroundColor: \"transparent\"")
        lines.append("---")
        lines.append("xychart-beta")
        lines.append('    title "Test Duration Percentiles"')
        lines.append('    x-axis ["P10", "P25", "P50", "P75", "P90", "P95", "P99", "P100"]')

        if self.test_cases:
            durations = sorted([t['duration'] for t in self.test_cases])
            percentiles = [
                durations[int(len(durations) * 0.1)],
                durations[int(len(durations) * 0.25)],
                durations[int(len(durations) * 0.5)],
                durations[int(len(durations) * 0.75)],
                durations[int(len(durations) * 0.9)],
                durations[int(len(durations) * 0.95)],
                durations[int(len(durations) * 0.99)],
                durations[-1]
            ]

            lines.append(f'    y-axis "Duration (seconds)" 0 --> {max(percentiles) + 5}')
            lines.append(f'    line [{", ".join(f"{p:.2f}" for p in percentiles)}]')

        lines.append("```")
        lines.append("")
        return lines

    def _generate_execution_timeline(self) -> List[str]:
        """Generate Gantt chart for test suite execution timeline."""
        lines = []
        lines.append("## Test Suite Execution Timeline")
        lines.append("")
        lines.append("```mermaid")
        lines.append("gantt")
        lines.append("    title Test Suite Execution Timeline (Top 15 Suites)")
        lines.append("    dateFormat YYYY-MM-DD")
        lines.append("    axisFormat %m-%d")
        lines.append("")

        # Sort suites by total time and take top 15
        sorted_suites = sorted([s for s in self.suites if s['total_time'] > 0],
                              key=lambda x: x['total_time'], reverse=True)[:15]

        # Use a base date for the timeline
        lines.append("    section Test Execution")

        for i, suite in enumerate(sorted_suites):
            name = suite['name'][:25] + '...' if len(suite['name']) > 25 else suite['name']
            # Clean name for Mermaid (remove special characters)
            name = name.replace(':', '').replace(',', '').replace('"', "'").replace('(', '').replace(')', '')

            # Calculate duration in days (1 second = 1 day for visualization)
            duration_days = max(1, int(suite['total_time']))

            if i == 0:
                lines.append(f"    {name}  :done, suite{i}, 2024-01-01, {duration_days}d")
            else:
                lines.append(f"    {name}  :done, suite{i}, after suite{i-1}, {duration_days}d")

        lines.append("```")
        lines.append("")

        # Add an alternative simpler timeline visualization
        lines.append("### Alternative Timeline View")
        lines.append("")
        lines.append("```mermaid")
        lines.append("---")
        lines.append("config:")
        lines.append("  themeVariables:")
        lines.append("    xyChart:")
        lines.append("      backgroundColor: \"transparent\"")
        lines.append("---")
        lines.append("xychart-beta horizontal")
        lines.append('    title "Suite Execution Timeline (cumulative seconds)"')

        # Create cumulative timeline data
        suite_names = []
        start_times = []
        durations = []
        cumulative = 0

        for suite in sorted_suites[:10]:  # Top 10 for clarity
            name = suite['name'][:15] + '...' if len(suite['name']) > 15 else suite['name']
            suite_names.append(f'"{name}"')
            start_times.append(f"{cumulative:.0f}")
            durations.append(f"{suite['total_time']:.0f}")
            cumulative += suite['total_time']

        lines.append(f'    x-axis [{", ".join(suite_names)}]')
        lines.append(f'    y-axis "Time (seconds)" 0 --> {cumulative + 100}')
        lines.append(f'    bar [{", ".join(durations)}]')

        lines.append("```")
        lines.append("")
        return lines

    def _generate_time_breakdown_pie(self) -> List[str]:
        """Generate pie chart for time breakdown."""
        lines = []
        lines.append("## Time Distribution Breakdown")
        lines.append("")
        lines.append("```mermaid")
        lines.append("pie title Time Distribution by Test Speed")

        # Calculate time spent in different speed categories
        fast_time = sum(t['duration'] for t in self.test_cases if t['duration'] < 1)
        medium_time = sum(t['duration'] for t in self.test_cases if 1 <= t['duration'] < 5)
        slow_time = sum(t['duration'] for t in self.test_cases if 5 <= t['duration'] < 20)
        very_slow_time = sum(t['duration'] for t in self.test_cases if t['duration'] >= 20)

        if fast_time > 0:
            lines.append(f'    "Fast (<1s)" : {fast_time:.1f}')
        if medium_time > 0:
            lines.append(f'    "Medium (1-5s)" : {medium_time:.1f}')
        if slow_time > 0:
            lines.append(f'    "Slow (5-20s)" : {slow_time:.1f}')
        if very_slow_time > 0:
            lines.append(f'    "Very Slow (>20s)" : {very_slow_time:.1f}')

        lines.append("```")
        lines.append("")

        # Add another pie chart for setup/test/teardown time
        lines.append("```mermaid")
        lines.append("pie title Time Distribution by Phase")
        lines.append(f'    "Test Execution" : {self.summary["total_test_time"]:.1f}')
        lines.append(f'    "BeforeSuite Setup" : {self.summary["total_before_suite"]:.1f}')
        lines.append(f'    "AfterSuite Teardown" : {self.summary["total_after_suite"]:.1f}')

        # Calculate unaccounted time (overhead)
        overhead = self.summary['total_suite_time'] - (self.summary['total_test_time'] +
                                                       self.summary['total_before_suite'] +
                                                       self.summary['total_after_suite'])
        if overhead > 0:
            lines.append(f'    "Framework Overhead" : {overhead:.1f}')

        lines.append("```")
        lines.append("")
        return lines

    def _generate_reproduction_instructions(self) -> List[str]:
        """Generate instructions for reproducing the report."""
        return [
            "<!-- ",
            "## How to Reproduce This Report",
            "",
            "### Prerequisites:",
            "- Python 3.6 or higher",
            "- Access to GitHub Actions logs",
            "",
            "### Steps:",
            "",
            "1. Download the test logs from GitHub Actions:",
            "   ```bash",
            "   # Using GitHub CLI",
            "   gh run view <RUN_ID> --log > test-logs-full.txt",
            "   ",
            "   # Or download from the GitHub Actions UI",
            "   # Or use the direct Azure blob storage URL if available",
            "   ```",
            "",
            "2. Run the analyzer script:",
            "   ```bash",
            "   # Basic usage - generates report to test-summary.md",
            "   python analyze_test_logs.py test-logs-full.txt",
            "   ",
            "   # Specify custom output file",
            "   python analyze_test_logs.py test-logs-full.txt -o custom-report.md",
            "   ",
            "   # Include GitHub URL and run time",
            "   python analyze_test_logs.py test-logs-full.txt \\",
            "     --github-url 'https://github.com/vmware-tanzu/vm-operator/actions/runs/17788435761/job/50560279183' \\",
            "     --run-time '57m 23s'",
            "   ",
            "   # Export parsed data to JSON",
            "   python analyze_test_logs.py test-logs-full.txt --json test-data.json",
            "   ",
            "   # Verbose output",
            "   python analyze_test_logs.py test-logs-full.txt -v",
            "   ",
            "   # Skip reproduction instructions in output",
            "   python analyze_test_logs.py test-logs-full.txt --no-instructions",
            "   ",
            "   # Include Mermaid diagrams (enabled by default)",
            "   python analyze_test_logs.py test-logs-full.txt --with-mermaid",
            "   ",
            "   # Disable Mermaid diagrams for plain markdown",
            "   python analyze_test_logs.py test-logs-full.txt --no-mermaid",
            "   ```",
            "",
            "### Command-line Options:",
            "- `log_file`: Path to the test log file (required)",
            "- `-o, --output`: Output markdown file (default: test-summary.md)",
            "- `-j, --json`: Export parsed data to JSON file",
            "- `--github-url`: GitHub Actions run URL",
            "- `--run-time`: Total run time (e.g., '57m 23s')",
            "- `--no-instructions`: Skip reproduction instructions in output",
            "- `--with-mermaid`: Include Mermaid diagrams (default: enabled)",
            "- `--no-mermaid`: Disable Mermaid diagrams",
            "- `-v, --verbose`: Enable verbose output",
            "-->",
            "",
            "# VM Operator Test Execution Report",
            ""
        ]

    def _generate_header(self) -> List[str]:
        """Generate report header."""
        lines = []
        lines.append("## GitHub Actions Run Summary")
        if self.github_url:
            lines.append(f"- **Run URL**: {self.github_url}")
        if self.run_time:
            lines.append(f"- **Total Execution Time**: {self.run_time}")
        lines.append(f"- **Status**: ✅ Success")
        lines.append("")
        return lines

    def _generate_overall_summary(self) -> List[str]:
        """Generate overall test summary."""
        lines = []
        lines.append("## Overall Test Summary")
        lines.append("")
        lines.append("| Metric | Value |")
        lines.append("|--------|-------|")
        lines.append(f"| Total Test Suites | {len(self.suites)} |")
        lines.append(f"| Total Test Cases | {len(self.test_cases)} |")
        lines.append(f"| Total Suite Execution Time | {self.summary['total_suite_time']:.2f}s ({self.summary['total_suite_time']/60:.2f}m) |")
        lines.append(f"| Total Test Case Time | {self.summary['total_test_time']:.2f}s ({self.summary['total_test_time']/60:.2f}m) |")
        lines.append(f"| Total BeforeSuite Time | {self.summary['total_before_suite']:.2f}s ({self.summary['total_before_suite']/60:.2f}m) |")
        lines.append(f"| Total AfterSuite Time | {self.summary['total_after_suite']:.2f}s ({self.summary['total_after_suite']/60:.2f}m) |")
        lines.append("")
        return lines

    def _generate_suite_timing_table(self) -> List[str]:
        """Generate test suites by execution time table."""
        lines = []
        lines.append("## Test Suites by Total Execution Time")
        lines.append("")
        lines.append("| Suite Name | Total Time | BeforeSuite | AfterSuite | Test Cases | Avg Test Time |")
        lines.append("|------------|------------|-------------|------------|------------|---------------|")

        sorted_suites = sorted([s for s in self.suites if s['total_time'] > 0],
                              key=lambda x: x['total_time'], reverse=True)
        for suite in sorted_suites[:25]:
            avg_test_time = sum(t['duration'] for t in suite['tests']) / len(suite['tests']) if suite['tests'] else 0
            suite_name = suite['name'][:50] + '...' if len(suite['name']) > 50 else suite['name']
            lines.append(f"| {suite_name} | {suite['total_time']:.2f}s | {suite['before_suite']:.2f}s | "
                        f"{suite['after_suite']:.2f}s | {suite['num_specs']} | {avg_test_time:.2f}s |")

        lines.append("")
        return lines

    def _generate_setup_teardown_tables(self) -> List[str]:
        """Generate setup/teardown timing tables."""
        lines = []
        lines.append("## Suites with Longest Setup/Teardown Times")
        lines.append("")
        lines.append("### Longest BeforeSuite Times")
        lines.append("")
        lines.append("| Suite Name | BeforeSuite Time |")
        lines.append("|------------|------------------|")

        sorted_before = sorted([s for s in self.suites if s['before_suite'] > 0],
                              key=lambda x: x['before_suite'], reverse=True)
        for suite in sorted_before[:10]:
            suite_name = suite['name'][:60] + '...' if len(suite['name']) > 60 else suite['name']
            lines.append(f"| {suite_name} | {suite['before_suite']:.2f}s |")

        lines.append("")
        lines.append("### Longest AfterSuite Times")
        lines.append("")
        lines.append("| Suite Name | AfterSuite Time |")
        lines.append("|------------|-----------------|")

        sorted_after = sorted([s for s in self.suites if s['after_suite'] > 0],
                             key=lambda x: x['after_suite'], reverse=True)
        for suite in sorted_after[:10]:
            suite_name = suite['name'][:60] + '...' if len(suite['name']) > 60 else suite['name']
            lines.append(f"| {suite_name} | {suite['after_suite']:.2f}s |")

        lines.append("")
        return lines

    def _generate_slowest_tests(self) -> List[str]:
        """Generate slowest test cases table."""
        lines = []
        lines.append("## Top 50 Slowest Test Cases")
        lines.append("")
        lines.append("| Test Name | Suite | Duration |")
        lines.append("|-----------|-------|----------|")

        sorted_tests = sorted(self.test_cases, key=lambda x: x['duration'], reverse=True)
        for test in sorted_tests[:50]:
            test_name = test['name'][:60] + '...' if len(test['name']) > 60 else test['name']
            suite_name = test['suite'][:40] + '...' if len(test['suite']) > 40 else test['suite']
            lines.append(f"| {test_name} | {suite_name} | {test['duration']:.2f}s |")

        lines.append("")
        return lines

    def _generate_duration_distribution(self) -> List[str]:
        """Generate test duration distribution table."""
        lines = []
        lines.append("## Test Duration Distribution")
        lines.append("")
        lines.append("| Duration Range | Count | Percentage |")
        lines.append("|----------------|-------|------------|")

        ranges = [
            (0, 1, "0-1s"),
            (1, 5, "1-5s"),
            (5, 10, "5-10s"),
            (10, 20, "10-20s"),
            (20, 30, "20-30s"),
            (30, 60, "30-60s"),
            (60, float('inf'), ">60s")
        ]

        for low, high, label in ranges:
            count = len([t for t in self.test_cases if low <= t['duration'] < high])
            percentage = (count / len(self.test_cases)) * 100 if self.test_cases else 0
            lines.append(f"| {label} | {count} | {percentage:.1f}% |")

        lines.append("")
        return lines

    def _generate_suite_breakdown(self) -> List[str]:
        """Generate test breakdown by suite."""
        lines = []
        lines.append("## Test Breakdown by Suite")
        lines.append("")
        lines.append("| Suite Name | Test Count | Total Test Time | Min | Max | Avg | Median |")
        lines.append("|------------|------------|-----------------|-----|-----|-----|--------|")

        suite_stats = []
        for suite in self.suites:
            if suite['tests']:
                durations = [t['duration'] for t in suite['tests']]
                durations.sort()
                suite_stats.append({
                    'name': suite['name'],
                    'count': len(suite['tests']),
                    'total': sum(durations),
                    'min': min(durations),
                    'max': max(durations),
                    'avg': sum(durations) / len(durations),
                    'median': durations[len(durations)//2]
                })

        suite_stats.sort(key=lambda x: x['total'], reverse=True)
        for stat in suite_stats[:30]:
            suite_name = stat['name'][:50] + '...' if len(stat['name']) > 50 else stat['name']
            lines.append(f"| {suite_name} | {stat['count']} | {stat['total']:.2f}s | "
                        f"{stat['min']:.2f}s | {stat['max']:.2f}s | {stat['avg']:.2f}s | {stat['median']:.2f}s |")

        lines.append("")
        return lines

    def _generate_fastest_tests(self) -> List[str]:
        """Generate fastest tests table."""
        lines = []
        lines.append("## Top 20 Fastest Tests (Examples of Well-Optimized Tests)")
        lines.append("")
        lines.append("| Test Name | Suite | Duration |")
        lines.append("|-----------|-------|----------|")

        fast_tests = sorted([t for t in self.test_cases if t['duration'] > 0.001],
                           key=lambda x: x['duration'])
        for test in fast_tests[:20]:
            test_name = test['name'][:60] + '...' if len(test['name']) > 60 else test['name']
            suite_name = test['suite'][:40] + '...' if len(test['suite']) > 40 else test['suite']
            lines.append(f"| {test_name} | {suite_name} | {test['duration']:.3f}s |")

        lines.append("")
        return lines

    def _generate_performance_insights(self) -> List[str]:
        """Generate performance insights section."""
        lines = []
        lines.append("## Performance Insights")
        lines.append("")
        lines.append("### Key Findings")
        lines.append("")

        if self.summary['total_suite_time'] > 0:
            overhead_pct = ((self.summary['total_before_suite'] + self.summary['total_after_suite']) /
                           self.summary['total_suite_time'] * 100)
            lines.append(f"- **Setup/Teardown Overhead**: {overhead_pct:.1f}% of total suite time")

        if self.test_cases:
            lines.append(f"- **Average Test Duration**: {(self.summary['total_test_time'] / len(self.test_cases)):.2f}s")

        before_suites = [s for s in self.suites if s['before_suite'] > 0]
        if before_suites:
            lines.append(f"- **Average BeforeSuite Time**: {(self.summary['total_before_suite'] / len(before_suites)):.2f}s")

        after_suites = [s for s in self.suites if s['after_suite'] > 0]
        if after_suites:
            lines.append(f"- **Average AfterSuite Time**: {(self.summary['total_after_suite'] / len(after_suites)):.2f}s")

        if self.test_cases:
            durations = sorted([t['duration'] for t in self.test_cases])
            lines.append("")
            lines.append("### Test Duration Percentiles")
            lines.append("")
            lines.append("| Percentile | Duration |")
            lines.append("|------------|----------|")
            lines.append(f"| P50 (Median) | {durations[int(len(durations) * 0.5)]:.2f}s |")
            lines.append(f"| P90 | {durations[int(len(durations) * 0.9)]:.2f}s |")
            lines.append(f"| P95 | {durations[int(len(durations) * 0.95)]:.2f}s |")
            lines.append(f"| P99 | {durations[int(len(durations) * 0.99)]:.2f}s |")

        lines.append("")
        lines.append("### Optimization Recommendations")
        lines.append("")
        lines.append("1. **Focus on slowest tests**: The top 50 slowest tests account for significant execution time")
        lines.append("2. **Optimize BeforeSuite/AfterSuite**: Some suites have setup times exceeding 10 seconds")
        lines.append("3. **Consider parallel execution**: Many test suites could potentially run in parallel")
        lines.append("4. **Review tests >20s**: These are candidates for optimization or splitting")
        lines.append("")

        return lines


def main():
    """Main entry point for the script."""
    parser = argparse.ArgumentParser(
        description='Analyze test logs and generate timing reports with optional Mermaid diagrams',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s test-logs.txt
  %(prog)s test-logs.txt -o report.md --json data.json
  %(prog)s test-logs.txt --github-url 'https://github.com/...' --run-time '57m 23s'
  %(prog)s test-logs.txt --with-mermaid -o report-with-charts.md
  %(prog)s test-logs.txt --no-mermaid  # Disable Mermaid diagrams
        """
    )

    parser.add_argument('log_file', help='Path to the test log file')
    parser.add_argument('-o', '--output', default='test-summary.md',
                       help='Output markdown file (default: test-summary.md)')
    parser.add_argument('-j', '--json', help='Export parsed data to JSON file')
    parser.add_argument('--github-url', help='GitHub Actions run URL')
    parser.add_argument('--run-time', help="Total run time (e.g., '57m 23s')")
    parser.add_argument('--no-instructions', action='store_true',
                       help='Skip reproduction instructions in output')
    parser.add_argument('--with-mermaid', dest='mermaid', action='store_true', default=True,
                       help='Include Mermaid diagrams in the report (default: enabled)')
    parser.add_argument('--no-mermaid', dest='mermaid', action='store_false',
                       help='Disable Mermaid diagrams in the report')
    parser.add_argument('-v', '--verbose', action='store_true',
                       help='Enable verbose output')

    args = parser.parse_args()

    # Check if log file exists
    if not Path(args.log_file).exists():
        print(f"Error: Log file '{args.log_file}' not found", file=sys.stderr)
        sys.exit(1)

    # Parse logs
    analyzer = TestLogAnalyzer(args.log_file, verbose=args.verbose)
    data = analyzer.parse_logs()

    # Export to JSON if requested
    if args.json:
        with open(args.json, 'w') as f:
            json.dump(data, f, indent=2, default=str)
        if args.verbose:
            print(f"Exported parsed data to {args.json}")

    # Generate report
    generator = ReportGenerator(data, github_url=args.github_url, run_time=args.run_time,
                               include_mermaid=args.mermaid)
    line_count = generator.generate_markdown(
        args.output,
        include_instructions=not args.no_instructions
    )

    print(f"✅ Report generated successfully: {args.output}")
    print(f"   - {len(data['suites'])} test suites analyzed")
    print(f"   - {len(data['test_cases'])} test cases processed")
    print(f"   - {line_count} lines written to report")
    if args.mermaid:
        print(f"   - Mermaid diagrams included")


if __name__ == '__main__':
    main()