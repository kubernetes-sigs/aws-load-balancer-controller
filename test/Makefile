# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You
# may not use this file except in compliance with the License. A copy of
# the License is located at
#
#       http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF
# ANY KIND, either express or implied. See the License for the specific
# language governing permissions and limitations under the License.

MAKEFILE_PATH = $(dir $(realpath -s $(firstword $(MAKEFILE_LIST))))

export GOPROXY = direct

build-test-binaries:
		rm -rf ${MAKEFILE_PATH}build
		mkdir -p ${MAKEFILE_PATH}build
		find . -name '*suite_test.go' -type f  | xargs dirname  | xargs ginkgo build
		find ${MAKEFILE_PATH} -name "*.test" -print0 | xargs -0 -I {} mv {} ${MAKEFILE_PATH}build
