#!/usr/bin/env python3
"""
Generate .pyroscope.yaml configuration file from Syft SBOM analysis.

This tool builds a Docker container, runs Syft to extract Java package information,
and generates a .pyroscope.yaml file mapping function names to file paths and GitHub repositories.
"""

import argparse
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path
from typing import Dict, List, Optional, Tuple
from urllib.parse import unquote

import requests
import yaml


class SyftParser:
    """Parse Syft JSON output to extract Java package information."""

    def __init__(self, syft_json: dict):
        self.syft_json = syft_json
        self.artifacts = syft_json.get("artifacts", [])

    def extract_java_packages(self) -> List[dict]:
        """Extract all Java packages from Syft output."""
        java_packages = []
        for artifact in self.artifacts:
            if artifact.get("type") == "java-archive" and artifact.get("language") == "java":
                java_packages.append(artifact)
        return java_packages

    def parse_maven_coordinates(self, artifact: dict) -> Optional[Tuple[str, str, str]]:
        """Parse Maven coordinates from artifact.
        
        Returns (groupId, artifactId, version) or None if not a Maven package.
        """
        purl = artifact.get("purl", "")
        if not purl.startswith("pkg:maven/"):
            return None
        
        # Parse pkg:maven/groupId/artifactId@version
        match = re.match(r"pkg:maven/([^@]+)@(.+)", purl)
        if not match:
            return None
        
        coords = match.group(1)
        version = match.group(2)
        
        # Split groupId/artifactId
        parts = coords.split("/")
        if len(parts) < 2:
            return None
        
        group_id = "/".join(parts[:-1])
        artifact_id = parts[-1]
        
        return (group_id, artifact_id, version)

    def get_main_application_jar(self) -> Optional[str]:
        """Identify the main application JAR file.
        
        The main application JAR is typically the one that doesn't have
        BOOT-INF/lib/ in its accessPath, or has the Main-Class manifest entry.
        """
        for artifact in self.artifacts:
            if artifact.get("type") != "java-archive":
                continue
            
            locations = artifact.get("locations", [])
            for location in locations:
                access_path = location.get("accessPath", "")
                path = location.get("path", "")
                
                # Main JAR doesn't have :BOOT-INF/lib/ in accessPath
                if ":BOOT-INF/lib/" not in access_path and path.endswith(".jar"):
                    # Check if it has Main-Class in manifest
                    metadata = artifact.get("metadata", {})
                    manifest = metadata.get("manifest", {})
                    main_entries = manifest.get("main", [])
                    for entry in main_entries:
                        if entry.get("key") == "Main-Class":
                            return path
        
        # Fallback: find JAR without BOOT-INF/lib/ in any location
        for artifact in self.artifacts:
            if artifact.get("type") != "java-archive":
                continue
            
            locations = artifact.get("locations", [])
            for location in locations:
                access_path = location.get("accessPath", "")
                if ":BOOT-INF/lib/" not in access_path:
                    return location.get("path")
        
        return None

    def is_application_code(self, artifact: dict, main_jar: Optional[str]) -> bool:
        """Determine if artifact is application code or a dependency."""
        if not main_jar:
            return False
        
        locations = artifact.get("locations", [])
        for location in locations:
            access_path = location.get("accessPath", "")
            path = location.get("path", "")
            
            # Application code is in the main JAR but not in BOOT-INF/lib/
            if path == main_jar and ":BOOT-INF/lib/" not in access_path:
                metadata = artifact.get("metadata", {})
                manifest = metadata.get("manifest", {})
                main_entries = manifest.get("main", [])
                
                # Check for Start-Class (Spring Boot) - this indicates application code
                for entry in main_entries:
                    if entry.get("key") == "Start-Class":
                        return True
                
                # Check Main-Class - exclude Spring Boot loader
                for entry in main_entries:
                    if entry.get("key") == "Main-Class":
                        main_class = entry.get("value", "")
                        # If Main-Class is Spring Boot loader, this is not application code
                        if "org.springframework.boot.loader" in main_class:
                            return False
                        # If Main-Class is in a non-standard package, it's likely application code
                        return True
                
                # If no Main-Class but in main JAR, check if it has Maven coordinates
                # Application code typically doesn't have Maven coordinates
                maven_coords = self.parse_maven_coordinates(artifact)
                if not maven_coords:
                    return True
        
        return False

    def extract_package_prefixes(self, artifact: dict) -> List[str]:
        """Extract Java package prefixes from artifact.
        
        For application code, we need to infer the package from the Start-Class
        (Spring Boot) or Main-Class. For dependencies, we use the groupId.
        """
        prefixes = []
        
        metadata = artifact.get("metadata", {})
        manifest = metadata.get("manifest", {})
        main_entries = manifest.get("main", [])
        
        # For Spring Boot apps, prefer Start-Class over Main-Class
        for entry in main_entries:
            if entry.get("key") == "Start-Class":
                start_class = entry.get("value", "")
                # Convert org.example.rideshare.Main to org/example/rideshare
                if "." in start_class:
                    package = ".".join(start_class.split(".")[:-1])
                    prefixes.append(package.replace(".", "/"))
                break
        
        # Fallback to Main-Class if no Start-Class
        if not prefixes:
            for entry in main_entries:
                if entry.get("key") == "Main-Class":
                    main_class = entry.get("value", "")
                    # Skip Spring Boot loader
                    if "org.springframework.boot.loader" not in main_class:
                        # Convert org.example.rideshare.Main to org/example/rideshare
                        if "." in main_class:
                            package = ".".join(main_class.split(".")[:-1])
                            prefixes.append(package.replace(".", "/"))
                    break
        
        # For dependencies, try to extract actual Java package prefixes from manifest
        # This is more accurate than inferring from groupId
        # Pass artifactId to help map to correct module paths
        maven_coords = self.parse_maven_coordinates(artifact)
        artifact_id = maven_coords[1] if maven_coords else None
        package_prefixes_from_manifest = self._extract_packages_from_manifest(manifest, artifact_id)
        if package_prefixes_from_manifest:
            prefixes.extend(package_prefixes_from_manifest)
        
        # Fallback: use groupId from Maven coordinates if no packages found in manifest
        if not prefixes:
            maven_coords = self.parse_maven_coordinates(artifact)
            if maven_coords:
                group_id, artifact_id, _ = maven_coords
                # Convert groupId to prefix format and extract parent prefixes
                group_prefixes = self._extract_group_id_prefixes(group_id)
                prefixes.extend(group_prefixes)
            
            # Also check pomProperties
            pom_props = metadata.get("pomProperties", {})
            if pom_props:
                group_id = pom_props.get("groupId", "")
                if group_id:
                    group_prefixes = self._extract_group_id_prefixes(group_id)
                    prefixes.extend(group_prefixes)
        
        return list(set(prefixes))  # Remove duplicates
    
    def _extract_packages_from_manifest(self, manifest: dict, artifact_id: Optional[str] = None) -> List[str]:
        """Extract Java package prefixes from manifest Export-Package or Import-Package.
        
        This is more accurate than inferring from groupId because it reflects
        the actual Java package structure in the JAR.
        
        Args:
            manifest: JAR manifest dictionary
            artifact_id: Maven artifactId (used to determine module-specific prefixes)
        
        Returns consolidated top-level package prefixes. For Spring Framework modules,
        returns module-specific prefixes like org/springframework/web, org/springframework/aop.
        """
        packages = set()
        main_entries = manifest.get("main", [])
        
        # Look for Export-Package or Import-Package in manifest
        for entry in main_entries:
            key = entry.get("key", "")
            if key in ["Export-Package", "Import-Package"]:
                value = entry.get("value", "")
                if value:
                    # Parse package list (format: "package1;version=...,package2;version=...")
                    parsed_packages = self._parse_manifest_package_list(value)
                    packages.update(parsed_packages)
        
        if not packages:
            return []
        
        # For multi-module projects, try to extract module-specific prefixes
        # This is generic and works for any library with module structure
        
        # Consolidate packages to find common root prefixes
        # For example, if we have org.apache.tomcat.util.*, org.apache.tomcat.websocket.*,
        # we should return org/apache/tomcat
        return self._consolidate_package_prefixes(list(packages))
    
    
    def _parse_manifest_package_list(self, package_list: str) -> List[str]:
        """Parse OSGi manifest package list.
        
        Format: "package1;version=1.0,package2;version=2.0;uses:=package3"
        Returns list of package names.
        """
        packages = []
        # Split by comma to get individual package entries
        entries = package_list.split(",")
        for entry in entries:
            # Extract package name (before first semicolon)
            # Handle cases where entry might have quotes or other formatting
            package = entry.split(";")[0].strip().strip('"').strip("'")
            # Validate it looks like a Java package name
            if package and "." in package and not package.startswith(("version", "uses", "[")):
                packages.append(package)
        return packages
    
    def _consolidate_package_prefixes(self, packages: List[str]) -> List[str]:
        """Consolidate package names to find common root prefixes.
        
        For example:
        - ['org.apache.tomcat.util', 'org.apache.tomcat.websocket'] -> ['org/apache/tomcat']
        - ['org.apache.tomcat', 'org.apache.catalina'] -> ['org/apache/tomcat', 'org/apache/catalina']
        """
        if not packages:
            return []
        
        # Group packages by their root (first 3 parts: org.apache.tomcat)
        package_groups = {}
        for package in packages:
            parts = package.split(".")
            if len(parts) >= 3:
                # Use first 3 parts as the root (e.g., org.apache.tomcat)
                root = ".".join(parts[:3])
                if root not in package_groups:
                    package_groups[root] = []
                package_groups[root].append(package)
            else:
                # For shorter packages, use the full package
                if package not in package_groups:
                    package_groups[package] = []
                package_groups[package].append(package)
        
        # For each group, determine the best prefix
        prefixes = []
        for root, group_packages in package_groups.items():
            if len(group_packages) == 1:
                # Single package, use it as-is
                prefix = group_packages[0].replace(".", "/")
                prefixes.append(prefix)
            else:
                # Multiple packages, find common prefix
                common_prefix = self._find_common_package_prefix(group_packages)
                if common_prefix:
                    prefixes.append(common_prefix.replace(".", "/"))
                else:
                    # Fallback: use the root
                    prefixes.append(root.replace(".", "/"))
        
        return sorted(set(prefixes))
    
    def _find_common_package_prefix(self, packages: List[str]) -> Optional[str]:
        """Find the longest common package prefix.
        
        For example:
        - ['org.apache.tomcat.util', 'org.apache.tomcat.websocket'] -> 'org.apache.tomcat'
        - ['org.apache.tomcat', 'org.apache.catalina'] -> 'org.apache'
        """
        if not packages:
            return None
        
        if len(packages) == 1:
            return packages[0]
        
        # Split all packages into parts
        parts_list = [pkg.split(".") for pkg in packages]
        
        # Find the minimum length
        min_len = min(len(parts) for parts in parts_list)
        
        # Find common prefix
        common_parts = []
        for i in range(min_len):
            # Check if all packages have the same part at position i
            first_part = parts_list[0][i]
            if all(parts[i] == first_part for parts in parts_list):
                common_parts.append(first_part)
            else:
                break
        
        if common_parts:
            return ".".join(common_parts)
        
        return None
    
    def _extract_group_id_prefixes(self, group_id: str) -> List[str]:
        """Extract prefixes from groupId, including parent prefixes.
        
        For example, 'org.apache.tomcat.embed' generates:
        - 'org/apache/tomcat/embed' (full)
        - 'org/apache/tomcat' (parent)
        - 'org/apache' (grandparent)
        
        This is important because Maven groupIds don't always match
        Java package structures (e.g., tomcat-embed has packages under org.apache.tomcat.*).
        """
        if not group_id:
            return []
        
        # Convert to prefix format
        prefix = group_id.replace(".", "/")
        prefixes = [prefix]
        
        # Extract parent prefixes (go up to 2 levels)
        parts = group_id.split(".")
        # Generate parent prefixes: org.apache.tomcat.embed -> org.apache.tomcat, org.apache
        for i in range(len(parts) - 1, max(0, len(parts) - 3), -1):
            parent_group = ".".join(parts[:i])
            parent_prefix = parent_group.replace(".", "/")
            prefixes.append(parent_prefix)
        
        return prefixes


class GitHubAPIClient:
    """Client for querying GitHub API to find repositories and source paths."""
    
    def __init__(self, token: Optional[str] = None, cache: Optional[Dict] = None, enabled: bool = True):
        """Initialize GitHub API client.
        
        Args:
            token: GitHub personal access token (optional, increases rate limit)
            cache: Optional cache dictionary to store API responses
            enabled: Whether to enable GitHub API calls (default: True)
        """
        self.enabled = enabled
        self.token = token or os.environ.get("GITHUB_TOKEN")
        self.cache = cache or {}
        self.base_url = "https://api.github.com"
        self.session = requests.Session()
        if self.token:
            self.session.headers.update({"Authorization": f"token {self.token}"})
        self.session.headers.update({
            "Accept": "application/vnd.github.v3+json",
            "User-Agent": "pyroscope-yaml-generator"
        })
        self.rate_limit_remaining = 60  # Default for unauthenticated
        self.rate_limit_reset = 0
    
    def _check_rate_limit(self):
        """Check and respect rate limits. Returns False if rate limited."""
        if self.rate_limit_remaining <= 0:
            wait_time = max(0, self.rate_limit_reset - time.time())
            if wait_time > 0:
                print(f"GitHub API rate limit reached. Skipping API calls (would wait {wait_time:.0f} seconds).", file=sys.stderr)
                return False
        return True
    
    def _make_request(self, url: str, params: Optional[Dict] = None) -> Optional[Dict]:
        """Make a GitHub API request with rate limit handling."""
        if not self._check_rate_limit():
            return None
        
        cache_key = f"{url}?{json.dumps(params or {}, sort_keys=True)}"
        if cache_key in self.cache:
            return self.cache[cache_key]
        
        try:
            response = self.session.get(url, params=params, timeout=5)
            
            # Update rate limit info
            self.rate_limit_remaining = int(response.headers.get("X-RateLimit-Remaining", 60))
            self.rate_limit_reset = int(response.headers.get("X-RateLimit-Reset", 0))
            
            if response.status_code == 404:
                return None
            if response.status_code == 403:
                # Rate limited - mark as exhausted
                self.rate_limit_remaining = 0
                return None
            response.raise_for_status()
            
            result = response.json()
            self.cache[cache_key] = result
            return result
        except requests.RequestException as e:
            # Silently fail - we'll fall back to other strategies
            return None
    
    def search_repository(self, query: str, owner_hint: Optional[str] = None) -> Optional[Tuple[str, str]]:
        """Search for a GitHub repository, prioritizing official repositories.
        
        Args:
            query: Search query (typically artifactId or repo name)
            owner_hint: Optional owner hint to prioritize results
        
        Returns:
            Tuple of (owner, repo) if found, None otherwise
        """
        if not self.enabled:
            return None
            
        # Try exact match first if we have owner hint
        # This is the most reliable way to get the official repo
        if owner_hint:
            url = f"{self.base_url}/repos/{owner_hint}/{query}"
            repo = self._make_request(url)
            if repo and not repo.get("fork", False):
                return (repo["owner"]["login"], repo["name"])
            # Also try common variations if exact match fails
            # Some repos have different names (e.g., spring-framework vs springframework)
            variations = [query.replace("-", ""), f"{query}-framework", f"{query}-project"]
            for variation in variations:
                url = f"{self.base_url}/repos/{owner_hint}/{variation}"
                repo = self._make_request(url)
                if repo and not repo.get("fork", False):
                    return (repo["owner"]["login"], repo["name"])
        
        # Search for repository with better query to find official repos
        search_url = f"{self.base_url}/search/repositories"
        # Search query: exact name match, exclude forks, Java language, with pom.xml
        # Excluding forks helps filter out unofficial copies
        search_query = f"{query} in:name language:java filename:pom.xml fork:false"
        params = {"q": search_query, "sort": "stars", "order": "desc", "per_page": 20}
        
        results = self._make_request(search_url, params)
        if not results or not results.get("items"):
            # Fallback: search without language/pom.xml filter, but still exclude forks
            params = {"q": f"{query} in:name fork:false", "sort": "stars", "order": "desc", "per_page": 20}
            results = self._make_request(search_url, params)
            if not results or not results.get("items"):
                # Last resort: allow forks but heavily penalize them in scoring
                params = {"q": f"{query} in:name", "sort": "stars", "order": "desc", "per_page": 20}
                results = self._make_request(search_url, params)
                if not results or not results.get("items"):
                    return None
        
        # Score and rank repositories to find the official one
        scored_repos = []
        for item in results["items"]:
            # Reject repos with very few stars (< 100) immediately
            # These are almost always copies/forks, not official repos
            # This applies to both User and Organization repos
            owner_type = item.get("owner", {}).get("type", "")
            stars = item.get("stargazers_count", 0)
            if stars < 100:
                continue  # Skip low-star repos entirely (both User and Organization)
            
            score = self._score_repository(item, query, owner_hint)
            if score > 0:
                scored_repos.append((score, item))
        
        # If we have results, check if top result might be a module in a framework repo
        # This handles cases like "spring-aop" which is a module in "spring-framework"
        if scored_repos:
            # Sort by score (highest first)
            scored_repos.sort(key=lambda x: x[0], reverse=True)
            best_repo = scored_repos[0][1]
            best_score = scored_repos[0][0]
            best_owner_type = best_repo.get("owner", {}).get("type", "")
            best_stars = best_repo.get("stargazers_count", 0)
            
            # If top result is still a personal repo (but >= 100 stars) or has relatively few stars,
            # also search for framework/parent repos that might contain this as a module
            if best_owner_type == "User" or (best_stars < 1000 and len(query.split('-')) > 1):
                # Try searching for a framework/parent repo that might contain this module
                # Extract base name (e.g., "spring" from "spring-aop")
                query_parts = query.split('-')
                if len(query_parts) > 1:
                    base_name = query_parts[0]
                    framework_queries = [
                        f"{base_name}-framework",  # spring-aop -> spring-framework
                        base_name,  # spring-aop -> spring
                    ]
                    
                    for fw_query in framework_queries:
                        if fw_query == query:  # Skip if same as original
                            continue
                        fw_results = self._make_request(search_url, {"q": f"{fw_query} in:name fork:false", "sort": "stars", "order": "desc", "per_page": 5})
                        if fw_results and fw_results.get("items"):
                            for fw_item in fw_results["items"]:
                                # Only consider organization-owned framework repos with many stars
                                fw_owner_type = fw_item.get("owner", {}).get("type", "")
                                fw_stars = fw_item.get("stargazers_count", 0)
                                if fw_owner_type == "Organization" and fw_stars >= 1000:
                                    # Score framework repo, but give bonus for being a framework/parent repo
                                    fw_score = self._score_repository(fw_item, query, owner_hint)
                                    fw_score += 500  # Significant bonus for popular org framework repos
                                    # If framework repo scores higher, prefer it
                                    if fw_score > best_score:
                                        return (fw_item["owner"]["login"], fw_item["name"])
            
            # Return the highest-scoring repository
            return (best_repo["owner"]["login"], best_repo["name"])
        
        # No valid results found
        return None
    
    def _score_repository(self, repo: dict, query: str, owner_hint: Optional[str] = None) -> int:
        """Score a repository to determine if it's the official one.
        
        Higher scores indicate more likely to be the official repository.
        
        Returns:
            Score (0 or negative means skip this repo)
        """
        score = 0
        
        # Skip archived repositories (usually not maintained)
        if repo.get("archived", False):
            return -100
        
        # Exact name match gets high priority
        if repo["name"].lower() == query.lower():
            score += 1000
        elif query.lower() in repo["name"].lower():
            score += 100
        
        # Prefer non-forks (official repos are rarely forks)
        if not repo.get("fork", False):
            score += 500
        else:
            # Forks get heavily penalized - almost never the official repo
            score -= 1000
        
        # Prefer organization-owned repos over personal repos
        owner_type = repo.get("owner", {}).get("type", "")
        owner_login = repo.get("owner", {}).get("login", "").lower()
        stars = repo.get("stargazers_count", 0)
        
        if owner_type == "Organization":
            score += 300
        elif owner_type == "User":
            # Personal repos get heavily penalized, especially with low stars
            # Very low star count (< 100) suggests it's likely a copy/fork, not official
            if stars < 100:
                score -= 500  # Heavy penalty for low-star personal repos
            # Personal repos get penalized, especially if owner hint suggests an org
            if owner_hint and owner_hint.lower() != owner_login:
                score -= 200  # Heavy penalty if owner doesn't match hint
            else:
                score -= 50
        
        # Prefer repos with more stars (indicates popularity/official status)
        # But give much higher weight to repos with significant star counts
        if stars >= 1000:
            score += 200  # High weight for very popular repos
        elif stars >= 100:
            score += min(stars, 1000) // 10  # Moderate weight
        else:
            score += stars // 20  # Low weight for repos with < 100 stars
        
        # Prefer repos that match owner hint
        if owner_hint and repo.get("owner", {}).get("login", "").lower() == owner_hint.lower():
            score += 200
        
        # Prefer repos with description (indicates maintained projects)
        if repo.get("description"):
            score += 50
        
        # Prefer repos that are not disabled
        if repo.get("disabled", False):
            score -= 500
        
        # Prefer repos with recent activity (updated_at)
        # This is a bonus, not a requirement
        
        return score
    
    def find_source_path(self, owner: str, repo: str, ref: str, artifact_id: str, group_id: str) -> Optional[str]:
        """Find the source code path in a GitHub repository.
        
        Args:
            owner: Repository owner
            repo: Repository name
            ref: Git reference (branch/tag)
            artifact_id: Maven artifactId
            group_id: Maven groupId
        
        Returns:
            Source path if found, None otherwise
        """
        if not self.enabled:
            return None
        # Check common locations
        common_paths = [
            "src/main/java",
            "java",
            f"{artifact_id}/src/main/java",
            f"src/{artifact_id}/main/java",
        ]
        
        # For multi-module projects, check if artifactId matches a module
        # First, try to get repository contents at root
        contents_url = f"{self.base_url}/repos/{owner}/{repo}/contents"
        root_contents = self._make_request(contents_url, {"ref": ref})
        
        if root_contents:
            # Look for pom.xml to identify Maven project
            has_pom = any(item["name"] == "pom.xml" for item in root_contents if item["type"] == "file")
            
            if has_pom:
                # Check if artifactId matches a directory (module)
                module_dirs = [item["name"] for item in root_contents 
                             if item["type"] == "dir" and not item["name"].startswith(".")]
                
                # Try exact match first
                if artifact_id in module_dirs:
                    module_path = f"{artifact_id}/src/main/java"
                    # Verify this path exists
                    module_contents = self._make_request(f"{contents_url}/{artifact_id}", {"ref": ref})
                    if module_contents:
                        if any(item["name"] == "src" for item in module_contents if item["type"] == "dir"):
                            return module_path
                
                # Try variations (spring-web -> spring-web, web -> spring-web)
                for module_dir in module_dirs:
                    if artifact_id in module_dir or module_dir in artifact_id:
                        module_path = f"{module_dir}/src/main/java"
                        module_contents = self._make_request(f"{contents_url}/{module_dir}", {"ref": ref})
                        if module_contents:
                            if any(item["name"] == "src" for item in module_contents if item["type"] == "dir"):
                                return module_path
                
                # Check for standard Maven structure at root
                if any(item["name"] == "src" for item in root_contents if item["type"] == "dir"):
                    return "src/main/java"
        
        # Fallback: check common paths
        for path in common_paths:
            path_contents = self._make_request(f"{contents_url}/{path}", {"ref": ref})
            if path_contents:
                return path
        
        return None
    
    def find_tag(self, owner: str, repo: str, version: str) -> Optional[str]:
        """Find the actual tag in repository that matches the version.
        
        Tries multiple common tag formats:
        - v{version} (e.g., v9.0.63)
        - {version} (e.g., 9.0.63)
        - release-{version}
        - {version}-release
        
        Args:
            owner: Repository owner
            repo: Repository name
            version: Maven version string
        
        Returns:
            Tag name if found, None otherwise
        """
        if not self.enabled:
            return None
        
        # Common tag patterns to try
        tag_patterns = [
            version,  # Try exact version first
            f"v{version}",  # Try v-prefixed
            f"release-{version}",
            f"{version}-release",
        ]
        
        # Get all tags from the repository (handle pagination)
        tags_url = f"{self.base_url}/repos/{owner}/{repo}/tags"
        all_tags = []
        page = 1
        per_page = 100
        
        while True:
            params = {"page": page, "per_page": per_page}
            tags = self._make_request(tags_url, params)
            # Check if tags is a list (success) or None/error
            if not tags or not isinstance(tags, list):
                break
            all_tags.extend(tags)
            # If we got fewer than per_page, we're done
            if len(tags) < per_page:
                break
            page += 1
            # Limit to first 3 pages to avoid too many API calls
            if page > 3:
                break
        
        if not all_tags:
            # If tag lookup fails, return None to fall back to default pattern
            return None
        
        # Create a set of tag names for fast lookup
        tag_names = {tag.get("name", "") for tag in all_tags}
        
        # Try each pattern in order
        for pattern in tag_patterns:
            if pattern in tag_names:
                return pattern
        
        # If no exact match, try case-insensitive match
        tag_names_lower = {name.lower(): name for name in tag_names}
        for pattern in tag_patterns:
            if pattern.lower() in tag_names_lower:
                return tag_names_lower[pattern.lower()]
        
        return None
    

class GitHubMapper:
    """Map Maven coordinates to GitHub repositories using GitHub API and metadata."""
    
    def __init__(self, mappings_file: Optional[str] = None, github_token: Optional[str] = None, enable_github_api: bool = True):
        self.mappings = {}
        # Mappings file is optional - only used as fallback
        if mappings_file and os.path.exists(mappings_file):
            with open(mappings_file, "r") as f:
                self.mappings = json.load(f)
        
        # Initialize GitHub API client - enabled by default if token is available
        # Token can come from parameter, environment variable, or zshell
        token = github_token or os.environ.get("GITHUB_TOKEN")
        api_enabled = enable_github_api and token is not None
        self.github_client = GitHubAPIClient(token=token, enabled=api_enabled)
    
    def find_mapping(self, artifact: dict, group_id: str, artifact_id: str, version: str) -> Optional[dict]:
        """Find GitHub repository mapping for Maven coordinates.
        
        Tries multiple strategies:
        1. Extract from POM metadata (URL, SCM) - most reliable
        2. Query GitHub API to find repository - generic approach
        3. Check explicit mappings file - optional fallback
        """
        # Strategy 1: Extract from POM metadata (most reliable)
        mapping = self._extract_from_pom_metadata(artifact, group_id, artifact_id, version)
        if mapping:
            # If path not found, try to infer from GitHub API
            if not mapping.get("path"):
                path = self._infer_path_from_github(artifact_id, group_id, mapping.get("owner"), mapping.get("repo"), mapping.get("ref"))
                if path:
                    mapping["path"] = path
                else:
                    mapping["path"] = self._infer_path_from_artifact(artifact_id, group_id)
            return mapping
        
        # Strategy 2: Query GitHub API to find repository (primary generic approach)
        mapping = self._infer_from_github_api(group_id, artifact_id, version)
        if mapping:
            return mapping
        
        # Strategy 3: Check explicit mappings file (optional fallback)
        mapping = self._check_explicit_mappings(group_id, artifact_id, version)
        if mapping:
            return mapping
        
        return None
    
    def _check_explicit_mappings(self, group_id: str, artifact_id: str, version: str) -> Optional[dict]:
        """Check explicit mappings from file."""
        # Check exact groupId match
        if group_id in self.mappings:
            mapping = self.mappings[group_id]
            owner = mapping["owner"]
            repo = mapping["repo"]
            
            path_mappings = mapping.get("path_mappings", {})
            if artifact_id in path_mappings:
                path = path_mappings[artifact_id]
            else:
                path = mapping.get("default_path", "")
            
            ref = self._version_to_ref(version, group_id, artifact_id)
            
            return {
                "owner": owner,
                "repo": repo,
                "ref": ref,
                "path": path
            }
        
        # Check for prefix matches (try longest match first)
        # Sort by length descending to match most specific prefix first
        sorted_mappings = sorted(self.mappings.items(), key=lambda x: len(x[0]), reverse=True)
        for mapped_group, mapping in sorted_mappings:
            if group_id.startswith(mapped_group):
                owner = mapping["owner"]
                repo = mapping["repo"]
                path_mappings = mapping.get("path_mappings", {})
                path = path_mappings.get(artifact_id, mapping.get("default_path", ""))
                ref = self._version_to_ref(version, mapped_group, artifact_id, owner, repo)
                
                return {
                    "owner": owner,
                    "repo": repo,
                    "ref": ref,
                    "path": path
                }
        
        return None
    
    def _extract_from_pom_metadata(self, artifact: dict, group_id: str, artifact_id: str, version: str) -> Optional[dict]:
        """Extract GitHub repository from POM metadata."""
        metadata = artifact.get("metadata", {})
        pom_project = metadata.get("pomProject", {})
        
        # Try to extract from URL
        url = pom_project.get("url", "")
        if url:
            github_info = self._parse_github_url(url)
            if github_info:
                owner, repo = github_info
                # Try to infer path from artifactId (check mappings first)
                path = self._infer_path_from_artifact(artifact_id, group_id)
                ref = self._version_to_ref(version, group_id, artifact_id, owner, repo)
                return {
                    "owner": owner,
                    "repo": repo,
                    "ref": ref,
                    "path": path
                }
        
        # Check parent POM if available
        parent = pom_project.get("parent", {})
        if parent:
            parent_group = parent.get("groupId", "")
            parent_artifact = parent.get("artifactId", "")
            # For multi-module projects, parent often has the repo info
            if parent_group and parent_artifact:
                # Try GitHub API on parent
                mapping = self._infer_from_github_api(parent_group, parent_artifact, version)
                if mapping:
                    # Adjust path for submodule
                    path = self._infer_path_from_github(artifact_id, group_id, mapping.get("owner"), mapping.get("repo"), mapping.get("ref"))
                    if not path:
                        path = self._infer_path_from_artifact(artifact_id, group_id)
                    mapping["path"] = path
                    return mapping
        
        return None
    
    def _parse_github_url(self, url: str) -> Optional[Tuple[str, str]]:
        """Parse GitHub owner/repo from URL."""
        if not url:
            return None
        
        # Match various GitHub URL patterns
        patterns = [
            r"github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$",
            r"git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$",
        ]
        
        for pattern in patterns:
            match = re.search(pattern, url)
            if match:
                return (match.group(1), match.group(2).rstrip('/'))
        
        return None
    
    def _infer_from_github_api(self, group_id: str, artifact_id: str, version: str) -> Optional[dict]:
        """Infer GitHub repository by querying GitHub API.
        
        This is a completely generic approach that searches GitHub for repositories
        matching the artifactId and determines the source path by inspecting
        the repository structure. No hardcoded organization or repository names.
        """
        # Extract owner hint only from groupId patterns that explicitly encode the owner
        # (e.g., io.github.username -> username, com.github.username -> username)
        owner_hint = None
        if group_id.startswith("io.github."):
            parts = group_id.split(".")
            if len(parts) >= 3:
                owner_hint = parts[2]
        elif group_id.startswith("com.github."):
            parts = group_id.split(".")
            if len(parts) >= 3:
                owner_hint = parts[2]
        
        # Search for repository using GitHub API
        # The scoring system will prioritize official repos (organizations, non-forks, etc.)
        repo_info = self.github_client.search_repository(artifact_id, owner_hint)
        if not repo_info:
            return None
        
        owner, repo = repo_info
        
        # Determine ref (tag/branch) from version - check actual tags in repo
        ref = self._version_to_ref(version, group_id, artifact_id, owner, repo)
        
        # Find source path by inspecting repository structure
        path = self.github_client.find_source_path(owner, repo, ref, artifact_id, group_id)
        
        # Fallback to generic path inference if API didn't find a path
        if not path:
            path = self._infer_path_from_artifact(artifact_id, group_id)
        
        return {
            "owner": owner,
            "repo": repo,
            "ref": ref,
            "path": path
        }
    
    def _infer_path_from_github(self, artifact_id: str, group_id: str, owner: Optional[str], repo: Optional[str], ref: Optional[str]) -> Optional[str]:
        """Infer source path using GitHub API if owner/repo/ref are available."""
        if owner and repo and ref:
            return self.github_client.find_source_path(owner, repo, ref, artifact_id, group_id)
        return None
    
    
    def _infer_path_from_artifact(self, artifact_id: str, group_id: str, explicit_path: Optional[str] = None) -> str:
        """Infer source path from artifactId and groupId.
        
        Args:
            artifact_id: Maven artifactId
            group_id: Maven groupId
            explicit_path: Path from explicit mappings (if available)
        
        Returns:
            Inferred source path
        """
        # If we have an explicit path from mappings file, use it
        if explicit_path:
            return explicit_path
        
        # Check explicit mappings for path information
        if group_id in self.mappings:
            mapping = self.mappings[group_id]
            path_mappings = mapping.get("path_mappings", {})
            if artifact_id in path_mappings:
                return path_mappings[artifact_id]
            default_path = mapping.get("default_path", "")
            if default_path:
                return default_path
        
        # Check for prefix matches in mappings
        sorted_mappings = sorted(self.mappings.items(), key=lambda x: len(x[0]), reverse=True)
        for mapped_group, mapping in sorted_mappings:
            if group_id.startswith(mapped_group):
                path_mappings = mapping.get("path_mappings", {})
                if artifact_id in path_mappings:
                    return path_mappings[artifact_id]
                default_path = mapping.get("default_path", "")
                if default_path:
                    return default_path
        
        # Default: assume standard Maven structure
        return "src/main/java"
    
    def _version_to_ref(self, version: str, group_id: str, artifact_id: str, owner: Optional[str] = None, repo: Optional[str] = None) -> str:
        """Convert Maven version to Git ref (tag/branch).
        
        Checks actual repository tags to determine the correct ref format.
        
        Args:
            version: Maven version string
            group_id: Maven groupId
            artifact_id: Maven artifactId
            owner: GitHub repository owner (optional, for tag checking)
            repo: GitHub repository name (optional, for tag checking)
        
        Returns:
            Git ref (tag or branch name)
        """
        # For versions ending in -SNAPSHOT, use main branch
        if version.endswith("-SNAPSHOT"):
            return "main"
        
        # For OpenJDK standard library, use specific ref
        if group_id in ["java", "sun", "javax"]:
            return "jdk-17+0"
        
        # If we have owner and repo, check actual tags in the repository
        if owner and repo and self.github_client.enabled:
            ref = self.github_client.find_tag(owner, repo, version)
            if ref:
                return ref
        
        # For semantic versions, try common patterns
        # Note: We try plain version first since many repos (like Tomcat) don't use v-prefix
        # If tag lookup failed, we'll fall back to trying plain version first
        if re.match(r'^\d+\.\d+', version):
            # Try plain version first (many repos like Tomcat use this)
            # If that doesn't work, caller can try v-prefixed
            return version
        
        # Default: use version as-is (often works for tags)
        return version


class PyroscopeYAMLGenerator:
    """Generate .pyroscope.yaml configuration file."""
    
    def __init__(self, github_mapper: GitHubMapper):
        self.github_mapper = github_mapper
        self.mappings = []
        # Track which prefixes have been used to prevent duplicates
        self.used_prefixes = {}  # prefix -> source_info
    
    def add_application_mapping(self, prefixes: List[str], local_path: str = "src/main/java"):
        """Add mapping for application code."""
        if not prefixes:
            return
        
        source_info = {"type": "local", "path": local_path}
        
        # Filter out prefixes that are already used
        new_prefixes = []
        for prefix in prefixes:
            if prefix not in self.used_prefixes:
                self.used_prefixes[prefix] = source_info
                new_prefixes.append(prefix)
            else:
                # Prefix already exists, skip it
                print(f"Warning: prefix '{prefix}' already mapped, skipping duplicate", file=sys.stderr)
        
        # Remove prefixes where one is a strict prefix of another
        new_prefixes = self._filter_nested_prefixes(new_prefixes)
        
        if new_prefixes:
            self.mappings.append({
                "function_name": [{"prefix": prefix} for prefix in sorted(new_prefixes)],
                "language": "java",
                "source": {
                    "local": {
                        "path": local_path
                    }
                }
            })
    
    def add_dependency_mapping(self, prefixes: List[str], artifact: dict, group_id: str, artifact_id: str, version: str):
        """Add mapping for dependency code."""
        if not prefixes:
            return
        
        # Try to find GitHub mapping
        github_mapping = self.github_mapper.find_mapping(artifact, group_id, artifact_id, version)
        
        if github_mapping:
            source_info = {
                "type": "github",
                "owner": github_mapping["owner"],
                "repo": github_mapping["repo"],
                "ref": github_mapping["ref"],
                "path": github_mapping["path"]
            }
            
            # Filter out prefixes that are already used
            new_prefixes = []
            for prefix in prefixes:
                if prefix not in self.used_prefixes:
                    self.used_prefixes[prefix] = source_info
                    new_prefixes.append(prefix)
                else:
                    # Prefix already exists, skip it
                    existing = self.used_prefixes[prefix]
                    print(f"Warning: prefix '{prefix}' already mapped to {existing}, skipping duplicate", file=sys.stderr)
            
            # Remove prefixes where one is a strict prefix of another
            # Keep only the most specific prefixes
            new_prefixes = self._filter_nested_prefixes(new_prefixes)
            
            if new_prefixes:
                # Group new prefixes by source (owner/repo/path) to create efficient mappings
                source_key = (github_mapping["owner"], github_mapping["repo"], github_mapping["path"], github_mapping["ref"])
                
                # Check if we already have a mapping for this exact source
                existing_mapping = None
                for mapping in self.mappings:
                    gh = mapping.get("source", {}).get("github", {})
                    if (gh.get("owner") == github_mapping["owner"] and
                        gh.get("repo") == github_mapping["repo"] and
                        gh.get("path") == github_mapping["path"] and
                        gh.get("ref") == github_mapping["ref"]):
                        existing_mapping = mapping
                        break
                
                if existing_mapping:
                    # Merge prefixes into existing mapping
                    existing_prefixes = {p["prefix"] for p in existing_mapping["function_name"]}
                    all_prefixes = sorted(existing_prefixes | set(new_prefixes))
                    # Filter out nested prefixes after merging
                    all_prefixes = self._filter_nested_prefixes(list(all_prefixes))
                    existing_mapping["function_name"] = [{"prefix": prefix} for prefix in all_prefixes]
                else:
                    # Create new mapping
                    self.mappings.append({
                        "function_name": [{"prefix": prefix} for prefix in sorted(new_prefixes)],
                        "language": "java",
                        "source": {
                            "github": github_mapping
                        }
                    })
        else:
            # Fallback: use local path (though this is less useful for dependencies)
            # Skip for now as we don't have reliable local paths for dependencies
            pass
    
    def _filter_nested_prefixes(self, prefixes: List[str]) -> List[str]:
        """Remove prefixes where one is a strict prefix of another.
        
        For example, if we have ['org/apache/tomcat', 'org/apache/tomcat/embed'],
        we should only keep 'org/apache/tomcat/embed' (the more specific one).
        """
        if not prefixes:
            return []
        
        # Sort by length (longest first) to check more specific prefixes first
        sorted_prefixes = sorted(prefixes, key=len, reverse=True)
        filtered = []
        
        for prefix in sorted_prefixes:
            # Check if this prefix is a strict prefix of any already added prefix
            is_nested = False
            for existing in filtered:
                # Check if prefix is a strict prefix of existing
                # (prefix must be shorter and existing must start with prefix + '/')
                if len(prefix) < len(existing) and existing.startswith(prefix + "/"):
                    is_nested = True
                    break
                # Check if existing is a strict prefix of prefix
                if len(existing) < len(prefix) and prefix.startswith(existing + "/"):
                    # Remove the less specific one and keep the more specific
                    filtered.remove(existing)
                    break
            
            if not is_nested:
                filtered.append(prefix)
        
        return sorted(filtered)
    
    def generate(self) -> dict:
        """Generate the complete YAML structure."""
        return {
            "source_code": {
                "mappings": self.mappings
            }
        }
    
    def write(self, output_path: str):
        """Write YAML to file."""
        yaml_content = self.generate()
        with open(output_path, "w") as f:
            yaml.dump(yaml_content, f, default_flow_style=False, sort_keys=False, allow_unicode=True)


def build_docker_image(dockerfile_path: str, image_name: str, context_dir: Optional[str] = None) -> bool:
    """Build Docker image from Dockerfile."""
    dockerfile_dir = os.path.dirname(os.path.abspath(dockerfile_path))
    context = context_dir if context_dir else dockerfile_dir
    
    print(f"Building Docker image '{image_name}' from {dockerfile_path}...")
    cmd = ["docker", "build", "-f", dockerfile_path, "-t", image_name, context]
    
    try:
        result = subprocess.run(cmd, check=True, capture_output=True, text=True)
        print("Docker image built successfully.")
        return True
    except subprocess.CalledProcessError as e:
        print(f"Error building Docker image: {e.stderr}", file=sys.stderr)
        return False


def run_syft(image_name: str) -> Optional[dict]:
    """Run Syft on Docker image and return JSON output."""
    print(f"Running Syft on image '{image_name}'...")
    cmd = ["syft", image_name, "-o", "json"]
    
    try:
        result = subprocess.run(cmd, check=True, capture_output=True, text=True)
        syft_json = json.loads(result.stdout)
        print("Syft analysis completed successfully.")
        return syft_json
    except subprocess.CalledProcessError as e:
        print(f"Error running Syft: {e.stderr}", file=sys.stderr)
        return None
    except json.JSONDecodeError as e:
        print(f"Error parsing Syft JSON output: {e}", file=sys.stderr)
        return None


def main():
    parser = argparse.ArgumentParser(
        description="Generate .pyroscope.yaml from Syft SBOM analysis"
    )
    parser.add_argument(
        "--dockerfile",
        required=True,
        help="Path to Dockerfile"
    )
    parser.add_argument(
        "--image-name",
        required=True,
        help="Docker image name to build"
    )
    parser.add_argument(
        "--output",
        default=".pyroscope.yaml",
        help="Output path for .pyroscope.yaml file (default: .pyroscope.yaml)"
    )
    parser.add_argument(
        "--context",
        help="Docker build context directory (default: Dockerfile directory)"
    )
    parser.add_argument(
        "--skip-build",
        action="store_true",
        help="Skip Docker build (use existing image)"
    )
    parser.add_argument(
        "--skip-syft",
        action="store_true",
        help="Skip Syft execution (use existing syft.json file)"
    )
    parser.add_argument(
        "--syft-json",
        help="Path to existing Syft JSON file (if --skip-syft is used)"
    )
    parser.add_argument(
        "--mappings-file",
        default=os.path.join(os.path.dirname(__file__), "java_repo_mappings.json"),
        help="Path to Java repository mappings JSON file (optional, used as fallback)"
    )
    parser.add_argument(
        "--github-token",
        help="GitHub personal access token (optional, increases API rate limit from 60 to 5000/hour). Can also be set via GITHUB_TOKEN environment variable."
    )
    
    args = parser.parse_args()
    
    # Load Syft JSON
    if args.skip_syft:
        if not args.syft_json:
            print("Error: --syft-json required when using --skip-syft", file=sys.stderr)
            sys.exit(1)
        with open(args.syft_json, "r") as f:
            syft_json = json.load(f)
    else:
        # Build Docker image
        if not args.skip_build:
            if not build_docker_image(args.dockerfile, args.image_name, args.context):
                sys.exit(1)
        
        # Run Syft
        syft_json = run_syft(args.image_name)
        if not syft_json:
            sys.exit(1)
    
    # Parse Syft output
    parser = SyftParser(syft_json)
    java_packages = parser.extract_java_packages()
    print(f"Found {len(java_packages)} Java packages")
    
    # Identify main application JAR
    main_jar = parser.get_main_application_jar()
    print(f"Main application JAR: {main_jar}")
    
    # Load GitHub mappings
    github_token = args.github_token or os.environ.get("GITHUB_TOKEN")
    github_mapper = GitHubMapper(args.mappings_file, github_token=github_token)
    
    # Generate YAML
    generator = PyroscopeYAMLGenerator(github_mapper)
    
    # Process packages - automatically detect application vs library code
    app_prefixes = []
    # Store full artifact info for dependencies to enable better mapping
    dependency_artifacts = []  # list of (artifact, group_id, artifact_id, version, prefixes)
    
    for package in java_packages:
        if parser.is_application_code(package, main_jar):
            prefixes = parser.extract_package_prefixes(package)
            app_prefixes.extend(prefixes)
        else:
            # Everything else is library/dependency code
            maven_coords = parser.parse_maven_coordinates(package)
            if maven_coords:
                group_id, artifact_id, version = maven_coords
                prefixes = parser.extract_package_prefixes(package)
                dependency_artifacts.append((package, group_id, artifact_id, version, prefixes))
            else:
                # Non-Maven package, still try to extract prefixes
                prefixes = parser.extract_package_prefixes(package)
                if prefixes:
                    # Try to infer from package structure
                    dependency_artifacts.append((package, None, None, None, prefixes))
    
    # Add application mapping
    if app_prefixes:
        # Filter out framework loader packages (generic check)
        filtered_prefixes = [
            prefix for prefix in set(app_prefixes)
            if not prefix.endswith("/loader") and "/loader/" not in prefix
        ]
        if filtered_prefixes:
            generator.add_application_mapping(filtered_prefixes)
    
    # Add dependency mappings - process all library code
    for artifact_info in dependency_artifacts:
        if len(artifact_info) == 5:
            package, group_id, artifact_id, version, prefixes = artifact_info
            unique_prefixes = list(set(prefixes))
            if unique_prefixes and group_id and artifact_id and version:
                generator.add_dependency_mapping(unique_prefixes, package, group_id, artifact_id, version)
    
    # Write output
    generator.write(args.output)
    print(f"Generated .pyroscope.yaml at {args.output}")


if __name__ == "__main__":
    main()

