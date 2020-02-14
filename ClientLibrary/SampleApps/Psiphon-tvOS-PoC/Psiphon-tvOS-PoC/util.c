/*
* Copyright (c) 2020, Psiphon Inc.
* All rights reserved.
*
* This program is free software: you can redistribute it and/or modify
* it under the terms of the GNU General Public License as published by
* the Free Software Foundation, either version 3 of the License, or
* (at your option) any later version.
*
* This program is distributed in the hope that it will be useful,
* but WITHOUT ANY WARRANTY; without even the implied warranty of
* MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
* GNU General Public License for more details.
*
* You should have received a copy of the GNU General Public License
* along with this program.  If not, see <http://www.gnu.org/licenses/>.
*
*/

#include "util.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

char *read_file(char *filename) {
    char *buffer = NULL;
    size_t size = 0;

    FILE *fp = fopen(filename, "r");

    if (!fp) {
        return NULL;
    }

    fseek(fp, 0, SEEK_END);
    size = ftell(fp);

    rewind(fp);
    buffer = malloc((size + 1) * sizeof(*buffer));

    fread(buffer, size, 1, fp);
    buffer[size] = '\0';

    return buffer;
}
